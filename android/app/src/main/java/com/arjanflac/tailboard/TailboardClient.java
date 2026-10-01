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
        final String id;
        final String content;
        final String source;
        final String deviceID;

        Clip(String id, String content, String source, String deviceID) {
            this.id = id;
            this.content = content;
            this.source = source;
            this.deviceID = deviceID;
        }

        static Clip fromJSON(JSONObject json) {
            return new Clip(
                    json.optString("id"),
                    json.optString("content"),
                    json.optString("source", "Mac"),
                    json.optString("device_id")
            );
        }
    }

    private static final MediaType JSON = MediaType.get("application/json; charset=utf-8");
    private final OkHttpClient http = new OkHttpClient.Builder()
            .connectTimeout(4, TimeUnit.SECONDS)
            .readTimeout(5, TimeUnit.SECONDS)
            .writeTimeout(5, TimeUnit.SECONDS)
            .pingInterval(25, TimeUnit.SECONDS)
            .callTimeout(6, TimeUnit.SECONDS)
            .retryOnConnectionFailure(false)
            .build();
    private final OkHttpClient streamHTTP = http.newBuilder()
            .readTimeout(0, TimeUnit.MILLISECONDS)
            .callTimeout(0, TimeUnit.MILLISECONDS)
            .retryOnConnectionFailure(true)
            .build();

    void probe(Context context, ResultCallback callback) {
        MacDestination destination = TailboardConfig.destination(context);
        if (!destination.configured()) {
            callback.onResult(false, "Choose a default Mac in Tailboard");
            return;
        }
        http.newCall(new Request.Builder().url(destination.url + "/healthz").build()).enqueue(new Callback() {
            @Override public void onFailure(Call call, java.io.IOException error) {
                callback.onResult(false, destination.unavailable());
            }
            @Override public void onResponse(Call call, Response response) {
                try (response) {
                    boolean ready = response.isSuccessful() && response.body() != null
                            && "ok".equals(response.body().string().trim());
                    callback.onResult(ready, ready ? "OK"
                            : "Tailboard not found on " + destination.name + ". Check its address.");
                } catch (java.io.IOException error) {
                    callback.onResult(false, destination.unavailable());
                }
            }
        });
    }

    void postText(Context context, String text, ClipCallback callback) {
        MacDestination destination = TailboardConfig.destination(context);
        if (!destination.configured()) {
            callback.onResult(null, "Choose a default Mac in Tailboard");
            return;
        }
        if (!MacDestination.validText(text)) {
            callback.onResult(null, text == null || text.isEmpty()
                    ? "Clipboard has no standalone text" : "Clipboard text is too large (maximum 1 MiB)");
            return;
        }
        try {
            JSONObject body = new JSONObject()
                    .put("content", text)
                    .put("device_id", TailboardConfig.deviceID(context));
            Request request = jsonRequest(context, destination, "/api/clip", body);
            http.newCall(request).enqueue(clipResponse(destination, callback));
        } catch (JSONException error) {
            callback.onResult(null, destination.unavailable());
        }
    }

    void getCurrent(Context context, ClipCallback callback) {
        MacDestination destination = TailboardConfig.destination(context);
        if (!destination.configured()) {
            callback.onResult(null, "Choose a default Mac in Tailboard");
            return;
        }
        Request request = new Request.Builder().url(destination.url + "/api/clip").build();
        http.newCall(request).enqueue(clipResponse(destination, callback));
    }

    void clear(Context context, ResultCallback callback) {
        MacDestination destination = TailboardConfig.destination(context);
        if (!destination.configured()) {
            callback.onResult(false, "Choose a default Mac in Tailboard");
            return;
        }
        Request request = new Request.Builder()
                .url(destination.url + "/api/clip")
                .delete()
                .build();
        execute(request, destination, callback);
    }

    WebSocket openStream(Context context, StreamListener listener) {
        MacDestination destination = TailboardConfig.destination(context);
        String httpURL = HttpUrl.get(destination.url + "/api/clip/stream").newBuilder()
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
                        listener.onDisconnected(destination.unavailable());
                    }
                });
    }

    private Callback clipResponse(MacDestination destination, ClipCallback callback) {
        return new Callback() {
            @Override public void onFailure(Call call, java.io.IOException error) {
                callback.onResult(null, destination.unavailable());
            }

            @Override public void onResponse(Call call, Response response) {
                try (response) {
                    if (response.code() == 204) {
                        callback.onResult(null, null);
                        return;
                    }
                    String body = response.body() == null ? "" : response.body().string();
                    if (!response.isSuccessful()) {
                        callback.onResult(null, destination.httpError(response.code()));
                        return;
                    }
                    Clip clip = Clip.fromJSON(new JSONObject(body));
                    if (clip.id.isEmpty() || clip.content.isEmpty()) throw new JSONException("Invalid clip");
                    callback.onResult(clip, null);
                } catch (Exception error) {
                    callback.onResult(null, "Unexpected reply from " + destination.name + ". Check its address.");
                }
            }
        };
    }

    private Request jsonRequest(Context context, MacDestination destination, String path, JSONObject body) {
        return new Request.Builder()
                .url(destination.url + path)
                .header("X-Clip-Source", TailboardConfig.deviceName(context))
                .post(RequestBody.create(body.toString(), JSON))
                .build();
    }

    private void execute(Request request, MacDestination destination, ResultCallback callback) {
        http.newCall(request).enqueue(new Callback() {
            @Override public void onFailure(Call call, java.io.IOException error) {
                callback.onResult(false, destination.unavailable());
            }

            @Override public void onResponse(Call call, Response response) {
                try (response) {
                    callback.onResult(response.isSuccessful(),
                            response.isSuccessful() ? "OK" : destination.httpError(response.code()));
                }
            }
        });
    }

}
