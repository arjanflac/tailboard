package com.arjanflac.tailboard;

import android.content.Context;

import org.json.JSONException;
import org.json.JSONObject;

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

final class TailboardClient {
    interface ResultCallback { void onResult(boolean success, String message); }
    interface ClipCallback { void onResult(Clip clip, String error); }
    interface StreamListener {
        void onConnected();
        void onClip(Clip clip);
        void onClear();
        void onDisconnected(String reason);
    }

    static final class Clip {
        final String content;
        final String source;
        final String deviceID;

        Clip(String content, String source, String deviceID) {
            this.content = content;
            this.source = source;
            this.deviceID = deviceID;
        }

        static Clip fromJSON(JSONObject json) {
            return new Clip(
                    json.optString("content"),
                    json.optString("source", "Mac"),
                    json.optString("device_id")
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
    private final OkHttpClient streamHTTP = http.newBuilder()
            .readTimeout(0, TimeUnit.MILLISECONDS)
            .build();

    void probe(Context context, ResultCallback callback) {
        execute(new Request.Builder().url(endpoint(context, "/healthz")).build(), callback);
    }

    void postText(Context context, String text, ClipCallback callback) {
        if (text == null || text.isEmpty()) {
            callback.onResult(null, "Clipboard has no standalone text");
            return;
        }
        try {
            JSONObject body = new JSONObject()
                    .put("content", text)
                    .put("device_id", TailboardConfig.deviceID(context));
            Request request = jsonRequest(context, "/api/clip", body);
            http.newCall(request).enqueue(clipResponse(callback));
        } catch (JSONException error) {
            callback.onResult(null, friendly(error));
        }
    }

    void getCurrent(Context context, ClipCallback callback) {
        Request request = new Request.Builder().url(endpoint(context, "/api/clip")).build();
        http.newCall(request).enqueue(clipResponse(callback));
    }

    void clear(Context context, ResultCallback callback) {
        Request request = new Request.Builder()
                .url(endpoint(context, "/api/clip"))
                .delete()
                .build();
        execute(request, callback);
    }

    WebSocket openStream(Context context, StreamListener listener) {
        String httpURL = HttpUrl.get(endpoint(context, "/api/clip/stream")).newBuilder()
                .addQueryParameter("device_id", TailboardConfig.deviceID(context))
                .build().toString();
        String socketURL = httpURL.startsWith("https://")
                ? "wss://" + httpURL.substring(8)
                : "ws://" + httpURL.substring(7);
        return streamHTTP.newWebSocket(new Request.Builder().url(socketURL).build(),
                new WebSocketListener() {
                    @Override public void onOpen(WebSocket socket, Response response) {
                        listener.onConnected();
                    }

                    @Override public void onMessage(WebSocket socket, String text) {
                        try {
                            JSONObject message = new JSONObject(text);
                            if ("clip_clear".equals(message.optString("type"))) {
                                listener.onClear();
                            } else if ("clip_update".equals(message.optString("type"))) {
                                JSONObject item = message.optJSONObject("item");
                                if (item != null) listener.onClip(Clip.fromJSON(item));
                            }
                        } catch (JSONException ignored) {}
                    }

                    @Override public void onClosed(WebSocket socket, int code, String reason) {
                        listener.onDisconnected(reason.isBlank() ? "Connection closed" : reason);
                    }

                    @Override public void onFailure(WebSocket socket, Throwable error, Response response) {
                        listener.onDisconnected(friendly(error));
                    }
                });
    }

    private Callback clipResponse(ClipCallback callback) {
        return new Callback() {
            @Override public void onFailure(Call call, java.io.IOException error) {
                callback.onResult(null, friendly(error));
            }

            @Override public void onResponse(Call call, Response response) {
                try (response) {
                    if (response.code() == 204) {
                        callback.onResult(null, null);
                        return;
                    }
                    String body = response.body() == null ? "" : response.body().string();
                    if (!response.isSuccessful()) {
                        callback.onResult(null, "Mac returned " + response.code());
                        return;
                    }
                    callback.onResult(Clip.fromJSON(new JSONObject(body)), null);
                } catch (Exception error) {
                    callback.onResult(null, friendly(error));
                }
            }
        };
    }

    private Request jsonRequest(Context context, String path, JSONObject body) {
        return new Request.Builder()
                .url(endpoint(context, path))
                .header("X-Clip-Source", TailboardConfig.deviceName(context))
                .post(RequestBody.create(body.toString(), JSON))
                .build();
    }

    private void execute(Request request, ResultCallback callback) {
        http.newCall(request).enqueue(new Callback() {
            @Override public void onFailure(Call call, java.io.IOException error) {
                callback.onResult(false, friendly(error));
            }

            @Override public void onResponse(Call call, Response response) {
                try (response) {
                    callback.onResult(response.isSuccessful(),
                            response.isSuccessful() ? "OK" : "Mac returned " + response.code());
                }
            }
        });
    }

    private static String endpoint(Context context, String path) {
        return TailboardConfig.serverURL(context) + path;
    }

    private static String friendly(Throwable error) {
        String message = error.getMessage();
        return message == null || message.isBlank() ? error.getClass().getSimpleName() : message;
    }
}
