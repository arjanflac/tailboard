package com.arjanflac.tailboard;

import android.content.BroadcastReceiver;
import android.content.Context;
import android.content.Intent;

public final class BootReceiver extends BroadcastReceiver {
    @Override
    public void onReceive(Context context, Intent intent) {
        String action = intent.getAction();
        if (!Intent.ACTION_BOOT_COMPLETED.equals(action)
                && !Intent.ACTION_MY_PACKAGE_REPLACED.equals(action)) {
            return;
        }
        try {
            ClipboardSyncService.start(context);
        } catch (RuntimeException ignored) {
            // Android may defer background starts; opening the app reconnects.
        }
    }
}
