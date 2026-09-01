package com.arjanflac.tgclipboard;

import android.app.Activity;
import android.content.Intent;
import android.os.Bundle;
import android.widget.Toast;

public final class ShareReceiverActivity extends Activity {
    @Override
    protected void onCreate(Bundle savedInstanceState) {
        super.onCreate(savedInstanceState);
        CharSequence text = getIntent().getCharSequenceExtra(Intent.EXTRA_TEXT);
        if (text != null && text.length() > 0) {
            ClipboardSyncService.send(this, text.toString());
        } else {
            Toast.makeText(this, "Nothing to send", Toast.LENGTH_SHORT).show();
        }
        finish();
    }
}
