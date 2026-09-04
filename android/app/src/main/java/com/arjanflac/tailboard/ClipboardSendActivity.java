package com.arjanflac.tailboard;

import android.app.Activity;
import android.graphics.PixelFormat;
import android.os.Build;
import android.os.Bundle;
import android.view.Gravity;
import android.view.WindowManager;
import android.widget.Toast;

/**
 * A transparent, user-launched activity used by the Quick Settings tile.
 * Android 10+ only exposes clipboard contents to the focused app, so the tile
 * cannot read them directly from its background TileService.
 */
public final class ClipboardSendActivity extends Activity {
    private boolean handled;

    @Override
    protected void onCreate(Bundle savedInstanceState) {
        super.onCreate(savedInstanceState);
        WindowManager.LayoutParams attributes = getWindow().getAttributes();
        attributes.dimAmount = 0f;
        attributes.width = 1;
        attributes.height = 1;
        attributes.x = -10_000;
        attributes.y = -10_000;
        attributes.gravity = Gravity.TOP | Gravity.START;
        attributes.format = PixelFormat.TRANSLUCENT;
        attributes.flags |= WindowManager.LayoutParams.FLAG_LAYOUT_NO_LIMITS
                | WindowManager.LayoutParams.FLAG_NOT_TOUCH_MODAL
                | WindowManager.LayoutParams.FLAG_ALT_FOCUSABLE_IM;
        attributes.softInputMode = WindowManager.LayoutParams.SOFT_INPUT_STATE_ALWAYS_HIDDEN
                | WindowManager.LayoutParams.SOFT_INPUT_ADJUST_NOTHING;
        getWindow().setAttributes(attributes);
        suppressTransitions();
    }

    @Override
    public void onWindowFocusChanged(boolean hasFocus) {
        super.onWindowFocusChanged(hasFocus);
        if (hasFocus && !handled) {
            handled = true;
            sendFocusedClipboard();
        }
    }

    private void sendFocusedClipboard() {
        String text = TextClipboard.read(this);
        if (text == null) {
            Toast.makeText(this, "Clipboard has no standalone text", Toast.LENGTH_SHORT).show();
            finishWithoutAnimation();
            return;
        }
        ClipboardSyncService.send(this, text);
        finishWithoutAnimation();
    }

    private void finishWithoutAnimation() {
        finish();
        suppressTransitions();
    }

    @SuppressWarnings("deprecation")
    private void suppressTransitions() {
        if (Build.VERSION.SDK_INT >= 34) {
            overrideActivityTransition(OVERRIDE_TRANSITION_OPEN, 0, 0);
            overrideActivityTransition(OVERRIDE_TRANSITION_CLOSE, 0, 0);
        } else {
            overridePendingTransition(0, 0);
        }
    }
}
