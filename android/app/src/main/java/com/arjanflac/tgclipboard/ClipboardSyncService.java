package com.arjanflac.tgclipboard;

import android.app.Notification;
import android.app.NotificationChannel;
import android.app.NotificationManager;
import android.app.PendingIntent;
import android.app.Service;
import android.content.ClipData;
import android.content.ClipboardManager;
import android.content.Context;
import android.content.Intent;
import android.os.Handler;
import android.os.IBinder;
import android.os.Looper;
import android.widget.Toast;

import java.util.concurrent.atomic.AtomicInteger;

import okhttp3.WebSocket;

public final class ClipboardSyncService extends Service {
    static final String ACTION_START = "com.arjanflac.tgclipboard.START";
    static final String ACTION_RESTART = "com.arjanflac.tgclipboard.RESTART";
    static final String ACTION_STOP = "com.arjanflac.tgclipboard.STOP";
    static final String ACTION_SEND_TEXT = "com.arjanflac.tgclipboard.SEND_TEXT";
    static final String ACTION_STATUS = "com.arjanflac.tgclipboard.STATUS";
    static final String EXTRA_TEXT = "text";
    static final String EXTRA_STATUS = "status";

    private static final String CHANNEL_ID = "clipboard_connection";
    private static final int NOTIFICATION_ID = 9437;

    private final Handler handler = new Handler(Looper.getMainLooper());
    private final AtomicInteger generation = new AtomicInteger();
    private final HubClient client = new HubClient();

    private ClipboardManager clipboard;
    private WebSocket stream;
    private int reconnectAttempt;
    private boolean stopped;
    private boolean foregroundStarted;

    static void start(Context context) {
        Intent intent = new Intent(context, ClipboardSyncService.class).setAction(ACTION_START);
        context.startForegroundService(intent);
    }

    static void restart(Context context) {
        Intent intent = new Intent(context, ClipboardSyncService.class).setAction(ACTION_RESTART);
        context.startForegroundService(intent);
    }

    static void send(Context context, String text) {
        Intent intent = new Intent(context, ClipboardSyncService.class)
                .setAction(ACTION_SEND_TEXT)
                .putExtra(EXTRA_TEXT, text);
        context.startForegroundService(intent);
    }

    @Override
    public void onCreate() {
        super.onCreate();
        clipboard = (ClipboardManager) getSystemService(CLIPBOARD_SERVICE);
        createNotificationChannel();
    }

    @Override
    public int onStartCommand(Intent intent, int flags, int startId) {
        String action = intent == null ? ACTION_START : intent.getAction();
        if (ACTION_STOP.equals(action)) {
            stopped = true;
            generation.incrementAndGet();
            if (stream != null) stream.close(1000, "Stopped");
            stopForeground(STOP_FOREGROUND_REMOVE);
            stopSelf();
            return START_NOT_STICKY;
        }

        stopped = false;
        if (!foregroundStarted) {
            startForeground(NOTIFICATION_ID, notification("Active through Tailscale"));
            foregroundStarted = true;
        }

        if (ACTION_SEND_TEXT.equals(action)) {
            sendText(intent.getStringExtra(EXTRA_TEXT));
        }
        if (ACTION_RESTART.equals(action)) {
            reconnectAttempt = 0;
            generation.incrementAndGet();
            if (stream != null) stream.close(1000, "Configuration changed");
            stream = null;
        }
        if (stream == null) {
            connect();
        }
        return START_STICKY;
    }

    private void connect() {
        final int thisGeneration = generation.incrementAndGet();
        updateStatus("Connecting to " + HubConfig.hubURL(this));
        client.register(this, (success, message) -> {
            if (stopped || thisGeneration != generation.get()) return;
            if (!success) {
                scheduleReconnect(thisGeneration, message);
                return;
            }
            stream = client.openStream(this, HubConfig.lastSequence(this), new HubClient.StreamListener() {
                @Override
                public void onConnected() {
                    if (thisGeneration != generation.get()) return;
                    reconnectAttempt = 0;
                    updateStatus("Connected through Tailscale");
                }

                @Override
                public void onClip(HubClient.Clip clip) {
                    if (thisGeneration != generation.get()) return;
                    HubConfig.setLastSequence(ClipboardSyncService.this, clip.sequence);
                    if (HubConfig.deviceID(ClipboardSyncService.this).equals(clip.deviceID)) return;
                    if (!clip.mimeType.startsWith("text/") || clip.content == null) {
                        updateStatus("Received unsupported " + clip.mimeType);
                        return;
                    }
                    handler.post(() -> {
                        String plainText = MainActivity.plainText(clip);
                        if (plainText == null) {
                            updateStatus("Received unsupported " + clip.mimeType);
                            return;
                        }
                        clipboard.setPrimaryClip(ClipData.newPlainText("Tailboard", plainText));
                        broadcastStatus("Received clipboard from " + clip.source);
                    });
                }

                @Override
                public void onDisconnected(String reason) {
                    scheduleReconnect(thisGeneration, reason);
                }
            });
        });
    }

    private void scheduleReconnect(int failedGeneration, String reason) {
        if (stopped || failedGeneration != generation.get()) return;
        stream = null;
        long delay = Math.min(60_000L, 1_000L << Math.min(reconnectAttempt++, 6));
        updateStatus("Reconnecting: " + reason);
        handler.postDelayed(() -> {
            if (!stopped && failedGeneration == generation.get() && stream == null) connect();
        }, delay);
    }

    private void sendText(String text) {
        client.postText(this, text, (success, message) -> handler.post(() -> {
            if (success) {
                broadcastStatus("Clipboard sent");
                Toast.makeText(this, "Clipboard sent", Toast.LENGTH_SHORT).show();
            } else {
                broadcastStatus("Send failed: " + message);
                Toast.makeText(this, "Send failed: " + message, Toast.LENGTH_LONG).show();
            }
        }));
    }

    private void updateStatus(String status) {
        handler.post(() -> broadcastStatus(status));
    }

    private void broadcastStatus(String status) {
        Intent broadcast = new Intent(ACTION_STATUS)
                .setPackage(getPackageName())
                .putExtra(EXTRA_STATUS, status);
        sendBroadcast(broadcast);
    }

    private Notification notification(String status) {
        Intent openIntent = new Intent(this, MainActivity.class);
        PendingIntent open = PendingIntent.getActivity(
                this, 1, openIntent, PendingIntent.FLAG_IMMUTABLE | PendingIntent.FLAG_UPDATE_CURRENT);
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

    private void createNotificationChannel() {
        NotificationChannel channel = new NotificationChannel(
                CHANNEL_ID, "Tailboard connection", NotificationManager.IMPORTANCE_LOW);
        channel.setDescription("Keeps Tailboard connected through Tailscale");
        getSystemService(NotificationManager.class).createNotificationChannel(channel);
    }

    @Override
    public void onDestroy() {
        stopped = true;
        generation.incrementAndGet();
        handler.removeCallbacksAndMessages(null);
        if (stream != null) stream.close(1000, "Service destroyed");
        stream = null;
        super.onDestroy();
    }

    @Override
    public IBinder onBind(Intent intent) {
        return null;
    }
}
