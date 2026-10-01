package com.arjanflac.tailboard;

import android.annotation.SuppressLint;
import android.app.PendingIntent;
import android.content.Intent;
import android.os.Build;
import android.os.Handler;
import android.os.Looper;
import android.service.quicksettings.Tile;
import android.service.quicksettings.TileService;

public final class ClipboardTileService extends TileService {
    private final Handler handler = new Handler(Looper.getMainLooper());
    private final TailboardClient client = new TailboardClient();
    private boolean listening;
    @Override
    public void onStartListening() {
        super.onStartListening();
        listening = true;
        MacDestination destination = TailboardConfig.destination(this);
        Tile tile = getQsTile();
        if (tile != null) {
            tile.setState(Tile.STATE_INACTIVE);
            tile.setLabel("Send Clipboard");
            tile.setSubtitle(destination.configured() ? destination.name : "Choose default Mac");
            tile.updateTile();
        }
        if (destination.configured()) client.probe(this, (success, message) -> handler.post(() -> {
            if (!listening || !destination.url.equals(TailboardConfig.serverURL(this))) return;
            Tile current = getQsTile();
            if (current != null) {
                current.setSubtitle(destination.name + (success ? " · Connected" : " · Unavailable"));
                current.updateTile();
            }
        }));
    }

    @Override public void onStopListening() {
        listening = false;
        super.onStopListening();
    }

    @Override
    @SuppressLint("StartActivityAndCollapseDeprecated")
    public void onClick() {
        super.onClick();
        unlockAndRun(() -> {
            Intent intent = new Intent(this, ClipboardSendActivity.class)
                    .addFlags(Intent.FLAG_ACTIVITY_NEW_TASK
                            | Intent.FLAG_ACTIVITY_CLEAR_TASK
                            | Intent.FLAG_ACTIVITY_NO_ANIMATION
                            | Intent.FLAG_ACTIVITY_NO_USER_ACTION);
            if (Build.VERSION.SDK_INT >= 34) {
                PendingIntent pendingIntent = PendingIntent.getActivity(
                        this,
                        9437,
                        intent,
                        PendingIntent.FLAG_IMMUTABLE | PendingIntent.FLAG_UPDATE_CURRENT);
                startActivityAndCollapse(pendingIntent);
            } else {
                startActivityAndCollapse(intent);
            }
        });
    }
}
