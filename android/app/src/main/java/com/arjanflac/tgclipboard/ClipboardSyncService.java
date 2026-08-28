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
import android.net.Uri;
import android.os.Build;
import android.os.Handler;
import android.os.IBinder;
import android.os.Looper;
import android.widget.Toast;

import java.util.concurrent.atomic.AtomicInteger;
import java.util.ArrayList;
import java.util.Collections;
import java.util.HashSet;
import java.util.List;
import java.util.Set;

import okhttp3.WebSocket;

public final class ClipboardSyncService extends Service {
    static final String ACTION_START = "com.arjanflac.tgclipboard.START";
    static final String ACTION_RESTART = "com.arjanflac.tgclipboard.RESTART";
    static final String ACTION_STOP = "com.arjanflac.tgclipboard.STOP";
    static final String ACTION_SEND_TEXT = "com.arjanflac.tgclipboard.SEND_TEXT";
    static final String ACTION_SEND_FILES = "com.arjanflac.tgclipboard.SEND_FILES";
    static final String ACTION_STATUS = "com.arjanflac.tgclipboard.STATUS";
    static final String EXTRA_TEXT = "text";
    static final String EXTRA_URIS = "uris";
    static final String EXTRA_TARGET_DEVICE = "target_device";
    static final String EXTRA_STATUS = "status";

    private static final String CHANNEL_ID = "clipboard_connection";
    private static final int NOTIFICATION_ID = 9437;

    private final Handler handler = new Handler(Looper.getMainLooper());
    private final AtomicInteger generation = new AtomicInteger();
    private final HubClient client = new HubClient();
    private final Set<String> receivingTransferIDs =
            Collections.synchronizedSet(new HashSet<>());

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

    static void sendFiles(Context context, List<Uri> uris) {
        sendFiles(context, uris, HubConfig.defaultTransferDevice(context));
    }

    static void sendFiles(Context context, List<Uri> uris, String targetDevice) {
        ArrayList<Uri> payload = new ArrayList<>(uris);
        Intent intent = new Intent(context, ClipboardSyncService.class)
                .setAction(ACTION_SEND_FILES)
                .putParcelableArrayListExtra(EXTRA_URIS, payload)
                .putExtra(EXTRA_TARGET_DEVICE, targetDevice)
                .addFlags(Intent.FLAG_GRANT_READ_URI_PERMISSION);
        if (!payload.isEmpty()) {
            ClipData grants = new ClipData(
                    "Shared files",
                    new String[]{"*/*"},
                    new ClipData.Item(payload.get(0))
            );
            for (int index = 1; index < payload.size(); index++) {
                grants.addItem(new ClipData.Item(payload.get(index)));
            }
            intent.setClipData(grants);
        }
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
        if (ACTION_SEND_FILES.equals(action)) {
            sendFiles(readSharedURIs(intent), intent.getStringExtra(EXTRA_TARGET_DEVICE));
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
                    client.listIncomingTransfers(
                            ClipboardSyncService.this,
                            ClipboardSyncService.this::handleIncomingTransfer
                    );
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
                public void onTransfer(HubClient.IncomingTransfer transfer) {
                    if (thisGeneration != generation.get()) return;
                    handleIncomingTransfer(transfer);
                }

                @Override
                public void onDisconnected(String reason) {
                    scheduleReconnect(thisGeneration, reason);
                }
            });
        });
    }

    private void handleIncomingTransfer(HubClient.IncomingTransfer transfer) {
        if (!HubConfig.deviceID(this).equals(transfer.toDevice)) return;
        if (!"offered".equals(transfer.state) && !"accepted".equals(transfer.state)) return;
        if (!receivingTransferIDs.add(transfer.transferID)) return;

        client.receiveTransferFromTrustedDevice(
                this,
                transfer,
                (success, message) -> handler.post(() -> {
                    receivingTransferIDs.remove(transfer.transferID);
                    if ("ignored".equals(message)) return;
                    broadcastStatus(message);
                    Toast.makeText(
                            this,
                            message,
                            success ? Toast.LENGTH_SHORT : Toast.LENGTH_LONG
                    ).show();
                })
        );
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

    @SuppressWarnings("deprecation")
    private List<Uri> readSharedURIs(Intent intent) {
        ArrayList<Uri> uris;
        if (Build.VERSION.SDK_INT >= 33) {
            uris = intent.getParcelableArrayListExtra(EXTRA_URIS, Uri.class);
        } else {
            uris = intent.getParcelableArrayListExtra(EXTRA_URIS);
        }
        return uris == null ? Collections.emptyList() : uris;
    }

    private void sendFiles(List<Uri> uris, String requestedTarget) {
        if (uris.isEmpty()) {
            Toast.makeText(this, "No files to send", Toast.LENGTH_SHORT).show();
            return;
        }
        String targetName = requestedTarget == null
                ? HubConfig.defaultTransferDevice(this)
                : requestedTarget.trim();
        if (targetName.isEmpty()) {
            Toast.makeText(this, "Choose a destination in Tailboard", Toast.LENGTH_LONG).show();
            return;
        }
        String friendlyTarget = HubConfig.friendlyTransferDevice(targetName);
        client.sendFilesToDeviceNamed(
                this,
                uris,
                targetName,
                (success, message) -> handler.post(() -> {
                    String confirmation = success
                            ? (uris.size() == 1 ? "File sent to " : "Files sent to ") + friendlyTarget
                            : message;
                    broadcastStatus(confirmation);
                    Toast.makeText(
                            getApplicationContext(),
                            confirmation,
                            Toast.LENGTH_LONG
                    ).show();
                })
        );
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
        receivingTransferIDs.clear();
        super.onDestroy();
    }

    @Override
    public IBinder onBind(Intent intent) {
        return null;
    }
}
