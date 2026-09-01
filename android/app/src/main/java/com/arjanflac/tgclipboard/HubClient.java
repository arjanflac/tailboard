package com.arjanflac.tgclipboard;

import android.content.Context;

import org.json.JSONArray;
import org.json.JSONException;
import org.json.JSONObject;

import java.util.ArrayList;
import java.util.List;
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
        void onDisconnected(String reason);
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

        Device(
                String deviceID,
                String name,
                String platform,
                boolean online
        ) {
            this.deviceID = deviceID;
            this.name = name;
            this.platform = platform;
            this.online = online;
        }

        static Device fromJSON(JSONObject json) {
            return new Device(
                    json.optString("device_id", ""),
                    json.optString("name", "Device"),
                    json.optString("platform", ""),
                    json.optBoolean("online")
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
            body.put("capabilities", new JSONArray().put("clipboard"));
            body.put("replace_capabilities", true);
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
