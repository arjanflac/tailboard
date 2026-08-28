package com.arjanflac.tgclipboard;

import android.content.ContentResolver;
import android.content.ContentValues;
import android.content.Context;
import android.database.Cursor;
import android.net.Uri;
import android.os.Environment;
import android.provider.MediaStore;
import android.provider.OpenableColumns;

import org.json.JSONArray;
import org.json.JSONException;
import org.json.JSONObject;

import java.io.IOException;
import java.io.InputStream;
import java.io.OutputStream;
import java.security.MessageDigest;
import java.util.ArrayList;
import java.util.List;
import java.util.Locale;
import java.util.concurrent.ExecutorService;
import java.util.concurrent.Executors;
import java.util.concurrent.TimeUnit;

import okhttp3.Call;
import okhttp3.Callback;
import okhttp3.HttpUrl;
import okhttp3.MediaType;
import okhttp3.OkHttpClient;
import okhttp3.Request;
import okhttp3.RequestBody;
import okhttp3.Response;
import okhttp3.WebSocket;
import okhttp3.WebSocketListener;
import okio.BufferedSink;
import okio.Okio;
import okio.Source;

final class HubClient {
    interface ResultCallback {
        void onResult(boolean success, String message);
    }

    interface ClipCallback {
        void onResult(Clip clip, String error);
    }

    interface DevicesCallback {
        void onResult(List<Device> devices, String error);
    }

    interface ClipsCallback {
        void onResult(List<Clip> clips, String error);
    }

    interface StreamListener {
        void onConnected();
        void onClip(Clip clip);
        void onTransfer(IncomingTransfer transfer);
        void onDisconnected(String reason);
    }

    interface TransferListener {
        void onTransfer(IncomingTransfer transfer);
    }

    static final class Clip {
        final long sequence;
        final String mimeType;
        final String content;
        final String source;
        final String deviceID;
        final String createdAt;

        Clip(
                long sequence,
                String mimeType,
                String content,
                String source,
                String deviceID,
                String createdAt
        ) {
            this.sequence = sequence;
            this.mimeType = mimeType;
            this.content = content;
            this.source = source;
            this.deviceID = deviceID;
            this.createdAt = createdAt;
        }

        static Clip fromJSON(JSONObject json) {
            return new Clip(
                    json.optLong("seq", 0L),
                    json.optString("mime_type", "text/plain"),
                    json.isNull("content") ? null : json.optString("content", null),
                    json.optString("source", "another device"),
                    json.optString("device_id", ""),
                    json.optString("created_at", "")
            );
        }
    }

    static final class Device {
        final String deviceID;
        final String name;
        final String platform;
        final boolean online;
        final boolean supportsTransfers;

        Device(
                String deviceID,
                String name,
                String platform,
                boolean online,
                boolean supportsTransfers
        ) {
            this.deviceID = deviceID;
            this.name = name;
            this.platform = platform;
            this.online = online;
            this.supportsTransfers = supportsTransfers;
        }

        static Device fromJSON(JSONObject json) {
            JSONArray capabilities = json.optJSONArray("capabilities");
            boolean supportsTransfers = false;
            if (capabilities != null) {
                for (int index = 0; index < capabilities.length(); index++) {
                    if ("transfers".equals(capabilities.optString(index))) {
                        supportsTransfers = true;
                        break;
                    }
                }
            }
            return new Device(
                    json.optString("device_id", ""),
                    json.optString("name", "Device"),
                    json.optString("platform", ""),
                    json.optBoolean("online"),
                    supportsTransfers
            );
        }
    }

    static final class IncomingFile {
        final String name;
        final long size;
        final String mimeType;
        final String sha256;

        IncomingFile(String name, long size, String mimeType, String sha256) {
            this.name = name;
            this.size = size;
            this.mimeType = mimeType;
            this.sha256 = sha256;
        }

        static IncomingFile fromJSON(JSONObject json) {
            return new IncomingFile(
                    json.optString("name", "Shared File"),
                    json.optLong("size", 0L),
                    json.optString("mime", "application/octet-stream"),
                    json.optString("sha256", "")
            );
        }
    }

    static final class IncomingTransfer {
        final String transferID;
        final String fromDevice;
        final String toDevice;
        final String state;
        final List<IncomingFile> files;

        IncomingTransfer(
                String transferID,
                String fromDevice,
                String toDevice,
                String state,
                List<IncomingFile> files
        ) {
            this.transferID = transferID;
            this.fromDevice = fromDevice;
            this.toDevice = toDevice;
            this.state = state;
            this.files = files;
        }

        static IncomingTransfer fromJSON(JSONObject json) throws JSONException {
            JSONArray fileArray = json.optJSONArray("files");
            List<IncomingFile> files = new ArrayList<>();
            if (fileArray != null) {
                for (int index = 0; index < fileArray.length(); index++) {
                    files.add(IncomingFile.fromJSON(fileArray.getJSONObject(index)));
                }
            }
            return new IncomingTransfer(
                    json.getString("transfer_id"),
                    json.optString("from_device", ""),
                    json.optString("to_device", ""),
                    json.optString("state", ""),
                    files
            );
        }
    }

    private static final MediaType JSON = MediaType.get("application/json; charset=utf-8");

    private final OkHttpClient http = new OkHttpClient.Builder()
            .connectTimeout(10, TimeUnit.SECONDS)
            .readTimeout(30, TimeUnit.SECONDS)
            .writeTimeout(15, TimeUnit.SECONDS)
            .pingInterval(25, TimeUnit.SECONDS)
            .retryOnConnectionFailure(true)
            .build();
    private final OkHttpClient webSocketHTTP = http.newBuilder()
            .readTimeout(0, TimeUnit.MILLISECONDS)
            .build();
    private final ExecutorService transferExecutor = Executors.newSingleThreadExecutor();

    void probe(Context context, ResultCallback callback) {
        Request request = new Request.Builder()
                .url(endpoint(context, "/healthz"))
                .get()
                .build();
        execute(request, callback);
    }

    void register(Context context, ResultCallback callback) {
        try {
            JSONObject body = new JSONObject();
            body.put("device_id", HubConfig.deviceID(context));
            body.put("name", HubConfig.deviceName(context));
            body.put("platform", "android");
            body.put("capabilities", new JSONArray().put("clipboard").put("transfers"));
            execute(jsonRequest(context, "/api/devices/register", body), callback);
        } catch (JSONException error) {
            callback.onResult(false, error.getMessage());
        }
    }

    void postText(Context context, String text, ResultCallback callback) {
        if (text == null || text.isEmpty()) {
            callback.onResult(false, "Clipboard is empty");
            return;
        }
        try {
            JSONObject body = new JSONObject();
            body.put("content", text);
            body.put("mime_type", "text/plain");
            body.put("device_id", HubConfig.deviceID(context));
            execute(jsonRequest(context, "/api/clip", body), callback);
        } catch (JSONException error) {
            callback.onResult(false, error.getMessage());
        }
    }

    void getCurrent(Context context, ClipCallback callback) {
        Request request = new Request.Builder()
                .url(endpoint(context, "/api/clip"))
                .get()
                .build();
        http.newCall(request).enqueue(new Callback() {
            @Override
            public void onFailure(Call call, java.io.IOException error) {
                callback.onResult(null, friendly(error));
            }

            @Override
            public void onResponse(Call call, Response response) {
                try (response) {
                    if (response.code() == 204) {
                        callback.onResult(null, "Nothing has been copied yet");
                        return;
                    }
                    String body = response.body() == null ? "" : response.body().string();
                    if (!response.isSuccessful()) {
                        callback.onResult(null, "Hub returned " + response.code());
                        return;
                    }
                    callback.onResult(Clip.fromJSON(new JSONObject(body)), null);
                } catch (Exception error) {
                    callback.onResult(null, friendly(error));
                }
            }
        });
    }

    void listDevices(Context context, DevicesCallback callback) {
        Request request = new Request.Builder()
                .url(endpoint(context, "/api/devices"))
                .get()
                .build();
        http.newCall(request).enqueue(new Callback() {
            @Override
            public void onFailure(Call call, java.io.IOException error) {
                callback.onResult(List.of(), friendly(error));
            }

            @Override
            public void onResponse(Call call, Response response) {
                try (response) {
                    String body = response.body() == null ? "" : response.body().string();
                    if (!response.isSuccessful()) {
                        callback.onResult(List.of(), "Hub returned " + response.code());
                        return;
                    }
                    JSONArray array = new JSONArray(body);
                    List<Device> devices = new ArrayList<>();
                    for (int index = 0; index < array.length(); index++) {
                        devices.add(Device.fromJSON(array.getJSONObject(index)));
                    }
                    callback.onResult(devices, null);
                } catch (Exception error) {
                    callback.onResult(List.of(), friendly(error));
                }
            }
        });
    }

    void getHistory(Context context, int limit, ClipsCallback callback) {
        HttpUrl url = HttpUrl.get(endpoint(context, "/api/clip/history"))
                .newBuilder()
                .addQueryParameter("limit", Integer.toString(limit))
                .build();
        Request request = new Request.Builder().url(url).get().build();
        http.newCall(request).enqueue(new Callback() {
            @Override
            public void onFailure(Call call, java.io.IOException error) {
                callback.onResult(List.of(), friendly(error));
            }

            @Override
            public void onResponse(Call call, Response response) {
                try (response) {
                    String body = response.body() == null ? "" : response.body().string();
                    if (!response.isSuccessful()) {
                        callback.onResult(List.of(), "Hub returned " + response.code());
                        return;
                    }
                    JSONArray array = new JSONArray(body);
                    List<Clip> clips = new ArrayList<>();
                    for (int index = 0; index < array.length(); index++) {
                        clips.add(Clip.fromJSON(array.getJSONObject(index)));
                    }
                    callback.onResult(clips, null);
                } catch (Exception error) {
                    callback.onResult(List.of(), friendly(error));
                }
            }
        });
    }

    WebSocket openStream(Context context, long sinceSequence, StreamListener listener) {
        HttpUrl base = HttpUrl.get(endpoint(context, "/api/clip/stream"));
        HttpUrl.Builder builder = base.newBuilder()
                .addQueryParameter("device_id", HubConfig.deviceID(context));
        if (sinceSequence > 0) {
            builder.addQueryParameter("since_seq", Long.toString(sinceSequence));
        }
        String httpURL = builder.build().toString();
        String webSocketURL = httpURL.startsWith("https://")
                ? "wss://" + httpURL.substring("https://".length())
                : "ws://" + httpURL.substring("http://".length());
        Request request = new Request.Builder().url(webSocketURL).build();
        return webSocketHTTP.newWebSocket(request, new WebSocketListener() {
            @Override
            public void onOpen(WebSocket webSocket, Response response) {
                listener.onConnected();
            }

            @Override
            public void onMessage(WebSocket webSocket, String text) {
                try {
                    JSONObject message = new JSONObject(text);
                    String type = message.optString("type");
                    if ("clip_update".equals(type)) {
                        JSONObject item = message.optJSONObject("item");
                        if (item != null) listener.onClip(Clip.fromJSON(item));
                    } else if ("transfer_offer".equals(type) || "transfer_state".equals(type)) {
                        JSONObject transfer = message.optJSONObject("transfer");
                        if (transfer != null) {
                            listener.onTransfer(IncomingTransfer.fromJSON(transfer));
                        }
                    }
                } catch (JSONException ignored) {
                    // Ignore malformed or future protocol messages.
                }
            }

            @Override
            public void onClosed(WebSocket webSocket, int code, String reason) {
                listener.onDisconnected(reason.isBlank() ? "Connection closed" : reason);
            }

            @Override
            public void onFailure(WebSocket webSocket, Throwable error, Response response) {
                listener.onDisconnected(friendly(error));
            }
        });
    }

    void listIncomingTransfers(Context context, TransferListener listener) {
        transferExecutor.execute(() -> {
            try {
                HttpUrl url = HttpUrl.get(endpoint(context, "/api/transfers"))
                        .newBuilder()
                        .addQueryParameter("device_id", HubConfig.deviceID(context))
                        .addQueryParameter("role", "receiver")
                        .build();
                Request request = new Request.Builder().url(url).get().build();
                try (Response response = http.newCall(request).execute()) {
                    String body = response.body() == null ? "" : response.body().string();
                    if (!response.isSuccessful()) throw new IOException("Hub returned " + response.code());
                    JSONArray transfers = new JSONArray(body);
                    for (int index = 0; index < transfers.length(); index++) {
                        IncomingTransfer transfer = IncomingTransfer.fromJSON(transfers.getJSONObject(index));
                        if ("offered".equals(transfer.state) || "accepted".equals(transfer.state)) {
                            listener.onTransfer(transfer);
                        }
                    }
                }
            } catch (Exception ignored) {
                // The live WebSocket remains the primary path. A later
                // reconnect retries this best-effort pending-offer recovery.
            }
        });
    }

    void receiveTransferFromTrustedDevice(
            Context context,
            IncomingTransfer transfer,
            ResultCallback callback
    ) {
        transferExecutor.execute(() -> {
            try {
                String localDeviceID = HubConfig.deviceID(context);
                if (!localDeviceID.equals(transfer.toDevice)) {
                    callback.onResult(false, "ignored");
                    return;
                }
                String sourcePlatform = findDevicePlatform(context, transfer.fromDevice);
                if (!"darwin".equalsIgnoreCase(sourcePlatform)
                        && !"ios".equalsIgnoreCase(sourcePlatform)) {
                    callback.onResult(false, "ignored");
                    return;
                }

                IncomingTransfer accepted = transfer;
                if ("offered".equals(accepted.state)) {
                    accepted = transferAction(context, accepted.transferID, "accept");
                }
                if (!"accepted".equals(accepted.state) && !"transferring".equals(accepted.state)) {
                    throw new IOException("Transfer is " + accepted.state);
                }
                for (int index = 0; index < accepted.files.size(); index++) {
                    downloadIncomingFile(context, accepted, index, accepted.files.get(index));
                }
                transferAction(context, accepted.transferID, "complete");
                callback.onResult(
                        true,
                        accepted.files.size() == 1
                                ? "File saved to Downloads/Tailboard"
                                : accepted.files.size() + " files saved to Downloads/Tailboard"
                );
            } catch (Exception error) {
                callback.onResult(false, friendly(error));
            }
        });
    }

    private String findDevicePlatform(Context context, String deviceID) throws Exception {
        Request request = new Request.Builder().url(endpoint(context, "/api/devices")).get().build();
        try (Response response = http.newCall(request).execute()) {
            String body = response.body() == null ? "" : response.body().string();
            if (!response.isSuccessful()) throw new IOException("Hub returned " + response.code());
            JSONArray devices = new JSONArray(body);
            for (int index = 0; index < devices.length(); index++) {
                JSONObject device = devices.getJSONObject(index);
                if (deviceID.equals(device.optString("device_id"))) {
                    return device.optString("platform", "");
                }
            }
        }
        throw new IOException("Sending device is no longer registered");
    }

    void sendFilesToDeviceNamed(
            Context context,
            List<Uri> uris,
            String targetName,
            ResultCallback callback
    ) {
        transferExecutor.execute(() -> {
            try {
                List<PreparedFile> files = new ArrayList<>();
                for (Uri uri : uris) files.add(prepareFile(context, uri));
                String targetDeviceID = findTransferDeviceID(context, targetName);
                JSONObject created = createTransfer(context, targetDeviceID, files);
                JSONArray uploadURLs = created.getJSONArray("upload_urls");
                if (uploadURLs.length() != files.size()) {
                    throw new IOException("Hub returned an incomplete upload manifest");
                }
                for (int index = 0; index < files.size(); index++) {
                    uploadFile(context, uploadURLs.getString(index), files.get(index));
                }
                callback.onResult(
                        true,
                        files.size() == 1
                                ? "File sent to " + targetName
                                : "Files sent to " + targetName
                );
            } catch (Exception error) {
                callback.onResult(false, friendly(error));
            }
        });
    }

    private String findTransferDeviceID(Context context, String targetName) throws Exception {
        Request request = new Request.Builder().url(endpoint(context, "/api/devices")).get().build();
        try (Response response = http.newCall(request).execute()) {
            String body = response.body() == null ? "" : response.body().string();
            if (!response.isSuccessful()) throw new IOException("Hub returned " + response.code());
            JSONArray devices = new JSONArray(body);
            String onlineDesktopID = null;
            for (int index = 0; index < devices.length(); index++) {
                JSONObject device = devices.getJSONObject(index);
                JSONArray capabilities = device.optJSONArray("capabilities");
                boolean supportsTransfers = false;
                if (capabilities != null) {
                    for (int capability = 0; capability < capabilities.length(); capability++) {
                        if ("transfers".equals(capabilities.optString(capability))) {
                            supportsTransfers = true;
                            break;
                        }
                    }
                }
                if (!supportsTransfers) continue;
                if (targetName.equalsIgnoreCase(device.optString("name").trim())) {
                    return device.getString("device_id");
                }
                if (device.optBoolean("online")
                        && "darwin".equalsIgnoreCase(device.optString("platform"))) {
                    onlineDesktopID = device.optString("device_id", onlineDesktopID);
                }
            }
            if (onlineDesktopID != null) return onlineDesktopID;
        }
        throw new IOException(targetName + " is not available for file transfers");
    }

    private JSONObject createTransfer(
            Context context,
            String targetDeviceID,
            List<PreparedFile> files
    ) throws Exception {
        JSONArray manifest = new JSONArray();
        for (PreparedFile file : files) {
            manifest.put(new JSONObject()
                    .put("name", file.name)
                    .put("size", file.size)
                    .put("mime", file.mimeType)
                    .put("sha256", file.sha256));
        }
        JSONObject body = new JSONObject()
                .put("to_device", targetDeviceID)
                .put("files", manifest);
        Request request = jsonRequest(context, "/api/transfers", body);
        try (Response response = http.newCall(request).execute()) {
            String responseBody = response.body() == null ? "" : response.body().string();
            if (!response.isSuccessful()) {
                throw new IOException("Transfer creation failed: " + response.code() + " " + responseBody);
            }
            return new JSONObject(responseBody);
        }
    }

    private void uploadFile(Context context, String uploadPath, PreparedFile file) throws Exception {
        String uploadURL = uploadPath.startsWith("http://") || uploadPath.startsWith("https://")
                ? uploadPath
                : HubConfig.hubURL(context) + uploadPath;
        RequestBody body = new RequestBody() {
            @Override
            public MediaType contentType() {
                return MediaType.parse(file.mimeType);
            }

            @Override
            public long contentLength() {
                return file.size;
            }

            @Override
            public void writeTo(BufferedSink sink) throws IOException {
                try (InputStream input = context.getContentResolver().openInputStream(file.uri)) {
                    if (input == null) throw new IOException("Can't open " + file.name);
                    try (Source source = Okio.source(input)) {
                        sink.writeAll(source);
                    }
                }
            }
        };
        Request request = new Request.Builder()
                .url(uploadURL)
                .header("X-Clip-Device-ID", HubConfig.deviceID(context))
                .header("Content-Range", "bytes 0-*/" + file.size)
                .put(body)
                .build();
        try (Response response = http.newCall(request).execute()) {
            if (!response.isSuccessful()) {
                String responseBody = response.body() == null ? "" : response.body().string();
                throw new IOException("File upload failed: " + response.code() + " " + responseBody);
            }
        }
    }

    private IncomingTransfer transferAction(Context context, String transferID, String action)
            throws Exception {
        Request request = new Request.Builder()
                .url(endpoint(context, "/api/transfers/" + transferID + "/" + action))
                .header("X-Clip-Device-ID", HubConfig.deviceID(context))
                .post(RequestBody.create("", JSON))
                .build();
        try (Response response = http.newCall(request).execute()) {
            String body = response.body() == null ? "" : response.body().string();
            if (!response.isSuccessful()) {
                throw new IOException("Transfer " + action + " failed: " + response.code());
            }
            return IncomingTransfer.fromJSON(new JSONObject(body));
        }
    }

    private void downloadIncomingFile(
            Context context,
            IncomingTransfer transfer,
            int index,
            IncomingFile file
    ) throws Exception {
        ContentResolver resolver = context.getContentResolver();
        ContentValues pending = new ContentValues();
        pending.put(MediaStore.MediaColumns.DISPLAY_NAME, safeFileName(file.name));
        pending.put(MediaStore.MediaColumns.MIME_TYPE, file.mimeType);
        pending.put(
                MediaStore.MediaColumns.RELATIVE_PATH,
                Environment.DIRECTORY_DOWNLOADS + "/Tailboard"
        );
        pending.put(MediaStore.MediaColumns.IS_PENDING, 1);
        Uri destination = resolver.insert(MediaStore.Downloads.EXTERNAL_CONTENT_URI, pending);
        if (destination == null) throw new IOException("Android could not create the download");

        boolean committed = false;
        try {
            Request request = new Request.Builder()
                    .url(endpoint(
                            context,
                            "/api/transfers/" + transfer.transferID + "/files/" + index
                    ))
                    .header("X-Clip-Device-ID", HubConfig.deviceID(context))
                    .get()
                    .build();
            try (Response response = http.newCall(request).execute()) {
                if (!response.isSuccessful() || response.body() == null) {
                    throw new IOException("Download failed: " + response.code());
                }
                MessageDigest digest = MessageDigest.getInstance("SHA-256");
                long written = 0L;
                try (InputStream input = response.body().byteStream();
                     OutputStream output = resolver.openOutputStream(destination, "w")) {
                    if (output == null) throw new IOException("Android could not open the download");
                    byte[] buffer = new byte[64 * 1024];
                    int read;
                    while ((read = input.read(buffer)) >= 0) {
                        if (read == 0) continue;
                        output.write(buffer, 0, read);
                        digest.update(buffer, 0, read);
                        written += read;
                    }
                }
                if (written != file.size) {
                    throw new IOException("Downloaded " + written + " of " + file.size + " bytes");
                }
                if (!hex(digest.digest()).equalsIgnoreCase(file.sha256)) {
                    throw new IOException("Downloaded file failed verification");
                }
            }

            ContentValues ready = new ContentValues();
            ready.put(MediaStore.MediaColumns.IS_PENDING, 0);
            if (resolver.update(destination, ready, null, null) != 1) {
                throw new IOException("Android could not publish the download");
            }
            committed = true;
        } finally {
            if (!committed) resolver.delete(destination, null, null);
        }
    }

    private static String safeFileName(String value) {
        String normalized = value == null ? "" : value.replace('\\', '/');
        int slash = normalized.lastIndexOf('/');
        if (slash >= 0) normalized = normalized.substring(slash + 1);
        normalized = normalized.replace("\u0000", "").trim();
        return normalized.isEmpty() || ".".equals(normalized) || "..".equals(normalized)
                ? "Shared File"
                : normalized;
    }

    private static String hex(byte[] bytes) {
        StringBuilder result = new StringBuilder(bytes.length * 2);
        for (byte value : bytes) {
            result.append(String.format(Locale.ROOT, "%02x", value & 0xff));
        }
        return result.toString();
    }

    private PreparedFile prepareFile(Context context, Uri uri) throws Exception {
        String name = null;
        try (Cursor cursor = context.getContentResolver().query(
                uri,
                new String[]{OpenableColumns.DISPLAY_NAME},
                null,
                null,
                null
        )) {
            if (cursor != null && cursor.moveToFirst()) {
                int column = cursor.getColumnIndex(OpenableColumns.DISPLAY_NAME);
                if (column >= 0 && !cursor.isNull(column)) name = cursor.getString(column);
            }
        }
        if (name == null || name.isBlank()) {
            String segment = uri.getLastPathSegment();
            name = segment == null || segment.isBlank() ? "Shared File" : segment;
        }
        String mimeType = context.getContentResolver().getType(uri);
        if (mimeType == null || mimeType.isBlank()) mimeType = "application/octet-stream";

        MessageDigest digest = MessageDigest.getInstance("SHA-256");
        long size = 0;
        try (InputStream input = context.getContentResolver().openInputStream(uri)) {
            if (input == null) throw new IOException("Can't open " + name);
            byte[] buffer = new byte[64 * 1024];
            int read;
            while ((read = input.read(buffer)) >= 0) {
                if (read == 0) continue;
                digest.update(buffer, 0, read);
                size += read;
            }
        }
        StringBuilder hash = new StringBuilder(64);
        for (byte value : digest.digest()) {
            hash.append(String.format(Locale.ROOT, "%02x", value & 0xff));
        }
        return new PreparedFile(uri, name, mimeType, size, hash.toString());
    }

    private static final class PreparedFile {
        final Uri uri;
        final String name;
        final String mimeType;
        final long size;
        final String sha256;

        PreparedFile(Uri uri, String name, String mimeType, long size, String sha256) {
            this.uri = uri;
            this.name = name;
            this.mimeType = mimeType;
            this.size = size;
            this.sha256 = sha256;
        }
    }

    private Request jsonRequest(Context context, String path, JSONObject body) {
        return new Request.Builder()
                .url(endpoint(context, path))
                .header("X-Clip-Source", HubConfig.deviceName(context))
                .header("X-Clip-Device-ID", HubConfig.deviceID(context))
                .post(RequestBody.create(body.toString(), JSON))
                .build();
    }

    private void execute(Request request, ResultCallback callback) {
        http.newCall(request).enqueue(new Callback() {
            @Override
            public void onFailure(Call call, java.io.IOException error) {
                callback.onResult(false, friendly(error));
            }

            @Override
            public void onResponse(Call call, Response response) {
                try (response) {
                    callback.onResult(response.isSuccessful(),
                            response.isSuccessful() ? "OK" : "Hub returned " + response.code());
                }
            }
        });
    }

    private static String endpoint(Context context, String path) {
        return HubConfig.hubURL(context) + path;
    }

    private static String friendly(Throwable error) {
        String message = error.getMessage();
        return message == null || message.isBlank() ? error.getClass().getSimpleName() : message;
    }
}
