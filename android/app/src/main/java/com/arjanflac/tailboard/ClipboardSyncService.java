package com.arjanflac.tailboard;

import android.app.Notification;
import android.app.NotificationChannel;
import android.app.NotificationManager;
import android.app.PendingIntent;
import android.app.Service;
import android.content.Context;
import android.content.Intent;
import android.content.ComponentName;
import android.service.quicksettings.TileService;
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
    static final String EXTRA_SERVER = "server";
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
    private MacDestination destination;
    private String connectionStatus = "Connecting…";

    static void start(Context context) {
        if (!TailboardConfig.destination(context).configured()) return;
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
        destination = TailboardConfig.destination(this);
        updates = new ClipboardUpdates(TailboardConfig.lastReceivedID(this));
        NotificationChannel channel = new NotificationChannel(
                CHANNEL_ID, "Tailboard connection", NotificationManager.IMPORTANCE_LOW);
        channel.setDescription("Keeps text connected to your Mac through Tailscale");
        getSystemService(NotificationManager.class).createNotificationChannel(channel);
    }

    @Override public int onStartCommand(Intent intent, int flags, int startID) {
        startForeground(NOTIFICATION_ID, notification(connectionStatus));
        stopped = false;
        String action = intent == null ? ACTION_START : intent.getAction();
        if (ACTION_RESTART.equals(action) || !destination.url.equals(TailboardConfig.serverURL(this))) {
            destination = TailboardConfig.destination(this);
            reconnectAttempt = 0;
            generation.incrementAndGet();
            if (stream != null) stream.cancel();
            stream = null;
        }
        if (ACTION_SEND_TEXT.equals(action)) sendText(intent.getStringExtra(EXTRA_TEXT));
        if (stream == null && destination.configured()) connect();
        if (!destination.configured()) updateConnectionStatus("Choose a default Mac in Tailboard");
        return START_STICKY;
    }

    private void connect() {
        int currentGeneration = generation.incrementAndGet();
        updateConnectionStatus("Connecting to " + destination.name + "…");
        stream = client.openStream(this, new TailboardClient.StreamListener() {
            @Override public void onConnected() {
                handler.post(() -> {
                    if (stopped || currentGeneration != generation.get()) return;
                    reconnectAttempt = 0;
                    updateConnectionStatus("Connected to " + destination.name);
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
        updateConnectionStatus(destination.name + " unavailable · reconnecting…");
        handler.postDelayed(() -> {
            if (!stopped && failedGeneration == generation.get() && stream == null) connect();
        }, delay);
    }

    private void sendText(String text) {
        MacDestination target = TailboardConfig.destination(this);
        client.postText(this, text, (clip, error) -> handler.post(() -> {
            String message = clip != null ? "Sent to " + target.name
                    : error == null ? "Unexpected reply from " + target.name : error;
            if (target.url.equals(destination.url)) updateStatus(message);
            Toast.makeText(this, message, clip != null ? Toast.LENGTH_SHORT : Toast.LENGTH_LONG).show();
        }));
    }

    private void updateConnectionStatus(String status) {
        connectionStatus = status;
        startForeground(NOTIFICATION_ID, notification(status));
        TileService.requestListeningState(this, new ComponentName(this, ClipboardTileService.class));
        updateStatus(status);
    }

    private void updateStatus(String status) {
        handler.post(() -> {
            Intent broadcast = new Intent(ACTION_STATUS)
                    .setPackage(getPackageName())
                    .putExtra(EXTRA_STATUS, status)
                    .putExtra(EXTRA_SERVER, destination.url);
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
