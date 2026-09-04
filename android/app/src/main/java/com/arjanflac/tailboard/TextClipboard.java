package com.arjanflac.tailboard;

import android.content.ClipData;
import android.content.ClipDescription;
import android.content.ClipboardManager;
import android.content.Context;

/** Reads literal text without coercing file URIs or intents into filenames. */
final class TextClipboard {
    private TextClipboard() {}

    static String read(Context context) {
        ClipboardManager clipboard = context.getSystemService(ClipboardManager.class);
        ClipData clip = clipboard.getPrimaryClip();
        ClipDescription description = clipboard.getPrimaryClipDescription();
        if (clip == null || clip.getItemCount() == 0 || description == null) return null;
        if (!description.hasMimeType("text/*")) return null;

        ClipData.Item item = clip.getItemAt(0);
        if (item.getUri() != null || item.getIntent() != null || item.getText() == null) return null;
        String text = item.getText().toString();
        return text.isEmpty() ? null : text;
    }

    static void write(Context context, String text) {
        context.getSystemService(ClipboardManager.class)
                .setPrimaryClip(ClipData.newPlainText("Tailboard", text));
    }

    static void clear(Context context) {
        context.getSystemService(ClipboardManager.class).clearPrimaryClip();
    }
}
