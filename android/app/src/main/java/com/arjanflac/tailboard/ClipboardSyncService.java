package com.arjanflac.tailboard;

import android.app.Notification;
import android.app.NotificationChannel;
import android.app.NotificationManager;
import android.app.PendingIntent;
import android.app.Service;
import android.content.Context;
import android.content.Intent;
import android.os.Handler;
import android.os.IBinder;
import android.os.Looper;
import android.widget.Toast;

import java.util.concurrent.atomic.AtomicInteger;

import okhttp3.WebSocket;

public final class ClipboardSyncService extends Service {
    static final String ACTION_START = "com.arjanflac.tailboard.START";
    static final String ACTION_RESTART = "com.arjanflac.tailboard.RESTART";
    static final String ACTION_SEND_TEXT = "com.arjanflac.tailboard.SEND_TEXT";
    static final String ACTION_STATUS = "com.arjanflac.tailboard.STATUS";
    static final String EXTRA_TEXT = "text";
    static final String EXTRA_STATUS = "status";

    private static final String CHANNEL_ID = "clipboard_connection";
    private static final int NOTIFICATION_ID = 9437;

    private final Handler handler = new Handler(Looper.getMainLooper());
    private final AtomicInteger generation = new AtomicInteger();
    private final TailboardClient client = new TailboardClient();
    private WebSocket stream;
    private ClipboardUpdates updates;
    private int reconnectAttempt;
    private boolean stopped;

    static void start(Context context) {
        context.startForegroundService(new Intent(context, ClipboardSyncService.class)
                .setAction(ACTION_START));
    }

    static void restart(Context context) {
        context.startForegroundService(new Intent(context, ClipboardSyncService.class)
                .setAction(ACTION_RESTART));
    }

    static void send(Context context, String text) {
        context.startForegroundService(new Intent(context, ClipboardSyncService.class)
                .setAction(ACTION_SEND_TEXT)
                .putExtra(EXTRA_TEXT, text));
    }

    @Override public void onCreate() {
        super.onCreate();
        updates = new ClipboardUpdates(TailboardConfig.lastReceivedID(this));
        NotificationChannel channel = new NotificationChannel(
                CHANNEL_ID, "Tailboard connection", NotificationManager.IMPORTANCE_LOW);
        channel.setDescription("Keeps text connected to your Mac through Tailscale");
        getSystemService(NotificationManager.class).createNotificationChannel(channel);
    }

    @Override public int onStartCommand(Intent intent, int flags, int startID) {
        startForeground(NOTIFICATION_ID, notification("Connected through Tailscale"));
        stopped = false;
        String action = intent == null ? ACTION_START : intent.getAction();
        if (ACTION_RESTART.equals(action)) {
            reconnectAttempt = 0;
            generation.incrementAndGet();
            if (stream != null) stream.cancel();
            stream = null;
        }
        if (ACTION_SEND_TEXT.equals(action)) sendText(intent.getStringExtra(EXTRA_TEXT));
        if (stream == null) connect();
        return START_STICKY;
    }

    private void connect() {
        int currentGeneration = generation.incrementAndGet();
        updateStatus("Connecting to Mac…");
        stream = client.openStream(this, new TailboardClient.StreamListener() {
            @Override public void onConnected() {
                handler.post(() -> {
                    if (stopped || currentGeneration != generation.get()) return;
                    reconnectAttempt = 0;
                    updateStatus("Connected through Tailscale");
                });
            }

            @Override public void onClip(TailboardClient.Clip clip) {
                handler.post(() -> {
                    if (stopped || currentGeneration != generation.get()) return;
                    if (TailboardConfig.deviceID(ClipboardSyncService.this).equals(clip.deviceID)) return;
                    if (updates.apply(clip.id, () -> TextClipboard.write(ClipboardSyncService.this, clip.content))) {
                        TailboardConfig.received(ClipboardSyncService.this, clip.id);
                        updateStatus("Copied from " + clip.source);
                    }
                });
            }

            @Override public void onClear() {
                handler.post(() -> {
                    if (stopped || currentGeneration != generation.get()) return;
                    TextClipboard.clear(ClipboardSyncService.this);
                    updateStatus("Clipboard cleared");
                });
            }

            @Override public void onDisconnected(String reason) {
                handler.post(() -> scheduleReconnect(currentGeneration));
            }
        });
    }

    private void scheduleReconnect(int failedGeneration) {
        if (stopped || failedGeneration != generation.get()) return;
        stream = null;
        long delay = Math.min(60_000L, 1_000L << Math.min(reconnectAttempt++, 6));
        updateStatus("Reconnecting…");
        handler.postDelayed(() -> {
            if (!stopped && failedGeneration == generation.get() && stream == null) connect();
        }, delay);
    }

    private void sendText(String text) {
        client.postText(this, text, (clip, error) -> handler.post(() -> {
            if (clip != null) {
                updateStatus("Sent to Mac");
                Toast.makeText(this, "Sent to Mac", Toast.LENGTH_SHORT).show();
            } else {
                updateStatus("Send failed");
                Toast.makeText(this, error, Toast.LENGTH_LONG).show();
            }
        }));
    }

    private void updateStatus(String status) {
        handler.post(() -> {
            Intent broadcast = new Intent(ACTION_STATUS)
                    .setPackage(getPackageName())
                    .putExtra(EXTRA_STATUS, status);
            sendBroadcast(broadcast);
        });
    }

    private Notification notification(String status) {
        PendingIntent open = PendingIntent.getActivity(this, 1,
                new Intent(this, MainActivity.class),
                PendingIntent.FLAG_IMMUTABLE | PendingIntent.FLAG_UPDATE_CURRENT);
        return new Notification.Builder(this, CHANNEL_ID)
                .setSmallIcon(R.drawable.ic_tailboard_mono)
                .setContentTitle("Tailboard")
                .setContentText(status)
                .setContentIntent(open)
                .setOngoing(true)
                .setOnlyAlertOnce(true)
                .setCategory(Notification.CATEGORY_SERVICE)
                .build();
    }

    @Override public void onDestroy() {
        stopped = true;
        generation.incrementAndGet();
        handler.removeCallbacksAndMessages(null);
        if (stream != null) stream.cancel();
        stream = null;
        super.onDestroy();
    }

    @Override public IBinder onBind(Intent intent) { return null; }
}
