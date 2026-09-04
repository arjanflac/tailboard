package com.arjanflac.tailboard;

import android.annotation.SuppressLint;
import android.app.Activity;
import android.app.StatusBarManager;
import android.content.BroadcastReceiver;
import android.content.ComponentName;
import android.content.Context;
import android.content.Intent;
import android.content.IntentFilter;
import android.content.pm.PackageManager;
import android.content.res.Configuration;
import android.graphics.Color;
import android.graphics.Typeface;
import android.graphics.drawable.GradientDrawable;
import android.graphics.drawable.Icon;
import android.os.Build;
import android.os.Bundle;
import android.os.Handler;
import android.os.Looper;
import android.text.TextUtils;
import android.view.Gravity;
import android.view.View;
import android.view.WindowInsets;
import android.widget.Button;
import android.widget.LinearLayout;
import android.widget.ScrollView;
import android.widget.TextView;
import android.widget.Toast;

public final class MainActivity extends Activity {
    private static final String ACTION_DEBUG_CONFIGURE =
            "com.arjanflac.tailboard.DEBUG_CONFIGURE";

    private final Handler handler = new Handler(Looper.getMainLooper());
    private final TailboardClient client = new TailboardClient();
    private TextView status;
    private TextView preview;
    private TextView source;
    private boolean dark;
    private int background;
    private int card;
    private int primary;
    private int secondary;

    private final BroadcastReceiver statusReceiver = new BroadcastReceiver() {
        @Override public void onReceive(Context context, Intent intent) {
            String value = intent.getStringExtra(ClipboardSyncService.EXTRA_STATUS);
            if (value != null) {
                status.setText(value);
                if (value.startsWith("Copied") || value.startsWith("Sent") || value.contains("cleared")) {
                    refresh();
                }
            }
        }
    };

    @Override protected void onCreate(Bundle savedInstanceState) {
        super.onCreate(savedInstanceState);
        palette();
        setContentView(build());
        if (ACTION_DEBUG_CONFIGURE.equals(getIntent().getAction())) {
            configureFromIntent();
            return;
        }
        ClipboardSyncService.start(this);
        client.probe(this, (success, message) -> handler.post(() ->
                status.setText(success ? "Connected through Tailscale" : "Mac unavailable")));
    }

    @Override protected void onResume() {
        super.onResume();
        refresh();
    }

    @SuppressLint("UnspecifiedRegisterReceiverFlag")
    @Override protected void onStart() {
        super.onStart();
        IntentFilter filter = new IntentFilter(ClipboardSyncService.ACTION_STATUS);
        if (Build.VERSION.SDK_INT >= 33) registerReceiver(statusReceiver, filter, RECEIVER_NOT_EXPORTED);
        else registerReceiver(statusReceiver, filter);
    }

    @Override protected void onStop() {
        unregisterReceiver(statusReceiver);
        super.onStop();
    }

    private void palette() {
        dark = (getResources().getConfiguration().uiMode & Configuration.UI_MODE_NIGHT_MASK)
                == Configuration.UI_MODE_NIGHT_YES;
        background = Color.parseColor(dark ? "#111111" : "#F3F3F1");
        card = Color.parseColor(dark ? "#1C1C1C" : "#FFFFFF");
        primary = Color.parseColor(dark ? "#F5F5F3" : "#1F1F1D");
        secondary = Color.parseColor(dark ? "#A4A49F" : "#6C6C67");
        getWindow().setStatusBarColor(background);
        getWindow().setNavigationBarColor(background);
        if (!dark) getWindow().getDecorView().setSystemUiVisibility(
                View.SYSTEM_UI_FLAG_LIGHT_STATUS_BAR | View.SYSTEM_UI_FLAG_LIGHT_NAVIGATION_BAR);
    }

    private View build() {
        ScrollView scroll = new ScrollView(this);
        scroll.setBackgroundColor(background);
        LinearLayout root = new LinearLayout(this);
        root.setOrientation(LinearLayout.VERTICAL);
        applySystemInsets(root);
        scroll.addView(root);

        TextView title = text("Tailboard", 30, primary);
        title.setTypeface(Typeface.DEFAULT, Typeface.BOLD);
        root.addView(title);
        TextView subtitle = text("Text between this phone and your Mac", 15, secondary);
        subtitle.setPadding(0, dp(4), 0, dp(16));
        root.addView(subtitle);
        status = text("Connecting…", 13, secondary);
        status.setPadding(dp(12), dp(9), dp(12), dp(9));
        status.setBackground(rounded(card, 12));
        root.addView(status);

        LinearLayout current = card();
        TextView label = text("CURRENT TEXT", 12, secondary);
        label.setTypeface(Typeface.DEFAULT, Typeface.BOLD);
        current.addView(label);
        preview = text("Nothing copied yet", 19, primary);
        preview.setMaxLines(7);
        preview.setEllipsize(TextUtils.TruncateAt.END);
        preview.setPadding(0, dp(12), 0, dp(8));
        current.addView(preview);
        source = text("", 13, secondary);
        current.addView(source);
        root.addView(current, cardParams(dp(24)));

        Button send = button("Send clipboard to Mac", true);
        send.setOnClickListener(view -> send());
        root.addView(send, blockParams(dp(14)));
        Button receive = button("Copy latest from Mac", false);
        receive.setOnClickListener(view -> receive());
        root.addView(receive, blockParams(dp(10)));
        Button clear = button("Clear both clipboards", false);
        clear.setOnClickListener(view -> clear());
        root.addView(clear, blockParams(dp(10)));

        TextView quickLabel = text("QUICK SETTINGS", 12, secondary);
        quickLabel.setTypeface(Typeface.DEFAULT, Typeface.BOLD);
        root.addView(quickLabel, blockParams(dp(28)));
        TextView quickHelp = text(
                "Add Send Clipboard for one-tap sending. Android only lets a foreground action read copied text.",
                14, secondary);
        quickHelp.setPadding(0, dp(8), 0, dp(4));
        root.addView(quickHelp);
        Button tile = button("Add Send Clipboard tile", false);
        tile.setOnClickListener(view -> requestTile());
        root.addView(tile, blockParams(dp(10)));

        TextView connectionLabel = text("CONNECTION", 12, secondary);
        connectionLabel.setTypeface(Typeface.DEFAULT, Typeface.BOLD);
        root.addView(connectionLabel, blockParams(dp(28)));
        LinearLayout connection = card();
        connection.addView(text("Mac\n" + TailboardConfig.serverURL(this), 14, primary));
        TextView device = text("\nThis phone\n" + TailboardConfig.deviceName(this), 14, primary);
        connection.addView(device);
        root.addView(connection, cardParams(dp(10)));
        Button settings = button("Edit connection", false);
        settings.setOnClickListener(view -> startActivity(new Intent(this, SettingsActivity.class)));
        root.addView(settings, blockParams(dp(10)));

        String version = "";
        try { version = getPackageManager().getPackageInfo(getPackageName(), 0).versionName; }
        catch (PackageManager.NameNotFoundException ignored) {}
        TextView footer = text("Text only · Taildrop handles files · v" + version, 12, secondary);
        footer.setGravity(Gravity.CENTER);
        root.addView(footer, blockParams(dp(30)));
        return scroll;
    }

    @SuppressWarnings("deprecation")
    private void applySystemInsets(View root) {
        int horizontal = dp(22);
        int top = dp(24);
        int bottom = dp(30);
        root.setPadding(horizontal, top, horizontal, bottom);
        root.setOnApplyWindowInsetsListener((view, insets) -> {
            int statusBar;
            int navigationBar;
            if (Build.VERSION.SDK_INT >= 30) {
                statusBar = insets.getInsets(WindowInsets.Type.statusBars()).top;
                navigationBar = insets.getInsets(WindowInsets.Type.navigationBars()).bottom;
            } else {
                statusBar = insets.getSystemWindowInsetTop();
                navigationBar = insets.getSystemWindowInsetBottom();
            }
            view.setPadding(horizontal, top + statusBar, horizontal, bottom + navigationBar);
            return insets;
        });
    }

    private void refresh() {
        if (preview == null) return;
        client.getCurrent(this, (clip, error) -> handler.post(() -> {
            if (clip == null) {
                preview.setText("Nothing copied yet");
                source.setText(error == null ? "" : error);
                preview.setOnClickListener(null);
            } else {
                preview.setText(clip.content);
                source.setText("From " + clip.source + " · tap to copy");
                preview.setOnClickListener(view -> {
                    TextClipboard.write(this, clip.content);
                    Toast.makeText(this, "Copied", Toast.LENGTH_SHORT).show();
                });
            }
        }));
    }

    private void send() {
        String text = TextClipboard.read(this);
        if (text == null) {
            Toast.makeText(this, "Clipboard has no standalone text", Toast.LENGTH_SHORT).show();
            return;
        }
        ClipboardSyncService.send(this, text);
    }

    private void receive() {
        client.getCurrent(this, (clip, error) -> handler.post(() -> {
            if (clip == null) {
                Toast.makeText(this, error == null ? "Nothing copied yet" : error, Toast.LENGTH_SHORT).show();
            } else {
                TextClipboard.write(this, clip.content);
                Toast.makeText(this, "Copied from Mac", Toast.LENGTH_SHORT).show();
                refresh();
            }
        }));
    }

    private void clear() {
        TextClipboard.clear(this);
        client.clear(this, (success, message) -> handler.post(() -> {
            Toast.makeText(this, success ? "Both clipboards cleared" : message, Toast.LENGTH_SHORT).show();
            refresh();
        }));
    }

    private void configureFromIntent() {
        String server = getIntent().getStringExtra("server_url");
        String name = getIntent().getStringExtra("device_name");
        if (server != null && name != null && !name.isBlank()) {
            TailboardConfig.save(this, server, name);
            ClipboardSyncService.restart(this);
        }
        finish();
    }

    private void requestTile() {
        if (Build.VERSION.SDK_INT < 33) {
            Toast.makeText(this, "Edit Quick Settings and drag in Send Clipboard", Toast.LENGTH_LONG).show();
            return;
        }
        getSystemService(StatusBarManager.class).requestAddTileService(
                new ComponentName(this, ClipboardTileService.class),
                "Send Clipboard", Icon.createWithResource(this, R.drawable.ic_tailboard_mono),
                getMainExecutor(), result -> {});
    }

    private LinearLayout card() {
        LinearLayout view = new LinearLayout(this);
        view.setOrientation(LinearLayout.VERTICAL);
        view.setPadding(dp(18), dp(17), dp(18), dp(17));
        view.setBackground(rounded(card, 18));
        return view;
    }

    private Button button(String label, boolean filled) {
        Button button = new Button(this);
        button.setText(label);
        button.setAllCaps(false);
        button.setTextSize(15);
        button.setTypeface(Typeface.DEFAULT, Typeface.BOLD);
        button.setTextColor(filled ? background : primary);
        button.setBackground(rounded(filled ? primary : card, 14));
        return button;
    }

    private TextView text(String value, int size, int color) {
        TextView view = new TextView(this);
        view.setText(value);
        view.setTextSize(size);
        view.setTextColor(color);
        view.setLineSpacing(0, 1.08f);
        return view;
    }

    private GradientDrawable rounded(int color, int radius) {
        GradientDrawable drawable = new GradientDrawable();
        drawable.setColor(color);
        drawable.setCornerRadius(dp(radius));
        return drawable;
    }

    private LinearLayout.LayoutParams blockParams(int top) {
        LinearLayout.LayoutParams params = new LinearLayout.LayoutParams(-1, dp(52));
        params.topMargin = top;
        return params;
    }

    private LinearLayout.LayoutParams cardParams(int top) {
        LinearLayout.LayoutParams params = new LinearLayout.LayoutParams(-1, -2);
        params.topMargin = top;
        return params;
    }

    private int dp(int value) {
        return Math.round(value * getResources().getDisplayMetrics().density);
    }
}
