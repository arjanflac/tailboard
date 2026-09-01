package com.arjanflac.tgclipboard;

import android.annotation.SuppressLint;
import android.app.Activity;
import android.app.StatusBarManager;
import android.content.BroadcastReceiver;
import android.content.ClipData;
import android.content.ClipboardManager;
import android.content.ComponentName;
import android.content.Context;
import android.content.Intent;
import android.content.IntentFilter;
import android.content.pm.ApplicationInfo;
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
import android.util.Log;
import android.view.Gravity;
import android.view.View;
import android.widget.Button;
import android.widget.FrameLayout;
import android.widget.LinearLayout;
import android.widget.ScrollView;
import android.widget.TextView;
import android.widget.Toast;

import java.util.ArrayList;
import java.util.List;
import java.util.Locale;

public final class MainActivity extends Activity {
    private static final String ACTION_DEBUG_PRIVACY_WIPE =
            "com.arjanflac.tgclipboard.DEBUG_PRIVACY_WIPE";
    private static final String ACTION_DEBUG_CONFIGURE =
            "com.arjanflac.tgclipboard.DEBUG_CONFIGURE";

    private final Handler handler = new Handler(Looper.getMainLooper());
    private final HubClient client = new HubClient();
    private final List<TextView> tabViews = new ArrayList<>();

    private FrameLayout contentHost;
    private TextView statusView;
    private int activeTab;
    private boolean privacyWipeRequested;

    private boolean dark;
    private int surface;
    private int card;
    private int primaryText;
    private int secondaryText;
    private int accent;
    private int separator;

    private final BroadcastReceiver statusReceiver = new BroadcastReceiver() {
        @Override
        public void onReceive(Context context, Intent intent) {
            String status = intent.getStringExtra(ClipboardSyncService.EXTRA_STATUS);
            if (status != null) {
                setStatus(
                        status,
                        status.startsWith("Connected")
                                || status.startsWith("Received")
                                || status.startsWith("Clipboard sent")
                );
                if (activeTab == 0 && (status.startsWith("Received") || status.startsWith("Clipboard sent"))) {
                    showTab(0);
                }
            }
        }
    };

    @Override
    protected void onCreate(Bundle savedInstanceState) {
        super.onCreate(savedInstanceState);
        configurePalette();
        privacyWipeRequested = (getApplicationInfo().flags & ApplicationInfo.FLAG_DEBUGGABLE) != 0
                && ACTION_DEBUG_PRIVACY_WIPE.equals(getIntent().getAction());
        boolean configureRequested =
                (getApplicationInfo().flags & ApplicationInfo.FLAG_DEBUGGABLE) != 0
                        && ACTION_DEBUG_CONFIGURE.equals(getIntent().getAction());
        setContentView(buildRoot());
        showTab(0);
        if (privacyWipeRequested) {
            setStatus("Clearing local clipboard…", true);
            performPrivacyWipe();
            return;
        }
        if (configureRequested) {
            performDebugConfiguration();
            return;
        }
        ClipboardSyncService.start(this);
        client.probe(this, (success, message) -> handler.post(() ->
                setStatus(success ? "Connected through Tailscale" : "Can't reach hub: " + message, success)));
    }

    @Override
    protected void onResume() {
        super.onResume();
        if (!privacyWipeRequested && contentHost != null) showTab(activeTab);
    }

    private void configurePalette() {
        dark = (getResources().getConfiguration().uiMode & Configuration.UI_MODE_NIGHT_MASK)
                == Configuration.UI_MODE_NIGHT_YES;
        surface = Color.parseColor(dark ? "#101114" : "#F5F5F7");
        card = Color.parseColor(dark ? "#1D1F22" : "#FFFFFF");
        primaryText = Color.parseColor(dark ? "#F9FAFB" : "#111827");
        secondaryText = Color.parseColor(dark ? "#A1A1AA" : "#6B7280");
        accent = Color.parseColor(dark ? "#60A5FA" : "#2563EB");
        separator = Color.parseColor(dark ? "#303238" : "#E5E7EB");
        getWindow().setStatusBarColor(surface);
        getWindow().setNavigationBarColor(surface);
        if (!dark) {
            getWindow().getDecorView().setSystemUiVisibility(
                    View.SYSTEM_UI_FLAG_LIGHT_STATUS_BAR | View.SYSTEM_UI_FLAG_LIGHT_NAVIGATION_BAR
            );
        }
    }

    private View buildRoot() {
        LinearLayout root = new LinearLayout(this);
        root.setOrientation(LinearLayout.VERTICAL);
        root.setBackgroundColor(surface);
        root.setOnApplyWindowInsetsListener((view, insets) -> {
            view.setPadding(
                    0,
                    insets.getSystemWindowInsetTop(),
                    0,
                    insets.getSystemWindowInsetBottom()
            );
            return insets;
        });

        LinearLayout header = new LinearLayout(this);
        header.setOrientation(LinearLayout.VERTICAL);
        header.setPadding(dp(22), dp(18), dp(22), dp(12));
        TextView brand = text("Tailboard", 29, primaryText);
        brand.setTypeface(Typeface.DEFAULT, Typeface.BOLD);
        header.addView(brand);
        TextView tagline = text("Clipboard sync over your tailnet", 14, secondaryText);
        tagline.setPadding(0, dp(3), 0, dp(12));
        header.addView(tagline);
        statusView = text("Connecting…", 13, Color.parseColor("#B45309"));
        statusView.setPadding(dp(12), dp(8), dp(12), dp(8));
        statusView.setBackground(rounded(Color.parseColor(dark ? "#3A2A12" : "#FFF7ED"), 10));
        header.addView(statusView, matchWrap());
        root.addView(header, matchWrap());

        contentHost = new FrameLayout(this);
        root.addView(contentHost, new LinearLayout.LayoutParams(
                LinearLayout.LayoutParams.MATCH_PARENT,
                0,
                1f
        ));

        LinearLayout tabs = new LinearLayout(this);
        tabs.setOrientation(LinearLayout.HORIZONTAL);
        tabs.setPadding(dp(10), dp(7), dp(10), dp(9));
        tabs.setBackgroundColor(card);
        addTab(tabs, "Clipboard", 0);
        addTab(tabs, "Devices", 1);
        addTab(tabs, "Settings", 2);
        root.addView(tabs, matchWrap());
        return root;
    }

    private void addTab(LinearLayout tabs, String label, int index) {
        TextView tab = text(label, 13, secondaryText);
        tab.setGravity(Gravity.CENTER);
        tab.setTypeface(Typeface.DEFAULT, Typeface.BOLD);
        tab.setPadding(dp(4), dp(11), dp(4), dp(11));
        tab.setOnClickListener(view -> showTab(index));
        tabs.addView(tab, new LinearLayout.LayoutParams(0, LinearLayout.LayoutParams.WRAP_CONTENT, 1f));
        tabViews.add(tab);
    }

    private void showTab(int tab) {
        activeTab = tab;
        for (int index = 0; index < tabViews.size(); index++) {
            TextView view = tabViews.get(index);
            boolean selected = index == tab;
            view.setTextColor(selected ? accent : secondaryText);
            view.setBackground(selected ? rounded(withAlpha(accent, 28), 12) : null);
        }
        contentHost.removeAllViews();
        View screen = tab == 0 ? clipboardScreen() : tab == 1 ? devicesScreen() : settingsScreen();
        contentHost.addView(screen, new FrameLayout.LayoutParams(
                FrameLayout.LayoutParams.MATCH_PARENT,
                FrameLayout.LayoutParams.MATCH_PARENT
        ));
    }

    private View clipboardScreen() {
        LinearLayout content = screenContent();
        addScreenTitle(content, "Clipboard", "The latest item is ready to copy on this phone.");

        LinearLayout currentCard = card();
        currentCard.addView(caption("CURRENT CLIP"));
        TextView currentPreview = text("Loading…", 18, primaryText);
        currentPreview.setMaxLines(5);
        currentPreview.setEllipsize(TextUtils.TruncateAt.END);
        currentPreview.setPadding(0, dp(10), 0, dp(8));
        currentCard.addView(currentPreview);
        TextView currentMeta = text("", 13, secondaryText);
        currentCard.addView(currentMeta);
        content.addView(currentCard, cardParams());

        LinearLayout actions = new LinearLayout(this);
        actions.setOrientation(LinearLayout.HORIZONTAL);
        Button send = actionButton("Send mine", true);
        send.setOnClickListener(view -> sendClipboard());
        Button receive = actionButton("Copy latest", false);
        receive.setOnClickListener(view -> receiveClipboard());
        actions.addView(send, new LinearLayout.LayoutParams(0, dp(50), 1f));
        LinearLayout.LayoutParams secondParams = new LinearLayout.LayoutParams(0, dp(50), 1f);
        secondParams.leftMargin = dp(10);
        actions.addView(receive, secondParams);
        content.addView(actions, spaced(dp(16)));

        content.addView(sectionLabel("Recent clips"));
        LinearLayout history = new LinearLayout(this);
        history.setOrientation(LinearLayout.VERTICAL);
        history.addView(secondaryMessage("Loading history…"));
        content.addView(history, matchWrap());

        client.getCurrent(this, (clip, error) -> handler.post(() -> {
            if (activeTab != 0 || currentPreview.getParent() == null) return;
            if (clip == null) {
                currentPreview.setText("Nothing copied yet");
                currentMeta.setText(error == null ? "Copy on any connected device" : error);
                currentCard.setOnClickListener(null);
                return;
            }
            LocalClipHistory.add(this, clip);
            renderLocalHistory(history);
            currentPreview.setText(clipPreview(clip));
            currentMeta.setText("From " + friendlySource(clip.source) + " · tap to copy");
            currentCard.setOnClickListener(view -> copyClip(clip));
        }));
        renderLocalHistory(history);
        return wrap(content);
    }

    private void renderLocalHistory(LinearLayout history) {
        history.removeAllViews();
        List<HubClient.Clip> recentClips = LocalClipHistory.load(this);
        if (recentClips.isEmpty()) {
            history.addView(secondaryMessage("No recent clips"));
        } else {
            for (HubClient.Clip clip : recentClips) history.addView(historyRow(clip));
        }
    }

    private View historyRow(HubClient.Clip clip) {
        LinearLayout row = card();
        row.setPadding(dp(15), dp(13), dp(15), dp(13));
        TextView preview = text(clipPreview(clip), 15, primaryText);
        preview.setMaxLines(2);
        preview.setEllipsize(TextUtils.TruncateAt.END);
        row.addView(preview);
        TextView meta = text("From " + friendlySource(clip.source), 12, secondaryText);
        meta.setPadding(0, dp(5), 0, 0);
        row.addView(meta);
        row.setOnClickListener(view -> copyClip(clip));
        LinearLayout.LayoutParams params = cardParams();
        params.topMargin = dp(8);
        row.setLayoutParams(params);
        return row;
    }

    private View devicesScreen() {
        LinearLayout content = screenContent();
        addScreenTitle(content, "Devices", "Everything currently registered with this Tailboard hub.");
        LinearLayout devices = new LinearLayout(this);
        devices.setOrientation(LinearLayout.VERTICAL);
        devices.addView(secondaryMessage("Loading devices…"));
        content.addView(devices, matchWrap());
        client.listDevices(this, (items, error) -> handler.post(() -> {
            if (activeTab != 1 || devices.getParent() == null) return;
            devices.removeAllViews();
            if (items.isEmpty()) {
                devices.addView(secondaryMessage(error == null ? "No devices found" : error));
                return;
            }
            items.sort((left, right) -> {
                if (left.online != right.online) return left.online ? -1 : 1;
                return left.name.compareToIgnoreCase(right.name);
            });
            for (HubClient.Device device : items) {
                LinearLayout row = card();
                row.setPadding(dp(15), dp(14), dp(15), dp(14));
                TextView name = text(device.name, 16, primaryText);
                name.setTypeface(Typeface.DEFAULT, Typeface.BOLD);
                row.addView(name);
                String state = device.online ? "Online" : "Offline";
                if (device.deviceID.equals(HubConfig.deviceID(this))) state += " · This device";
                TextView detail = text(
                        friendlyPlatform(device.platform) + " · " + state,
                        13,
                        device.online ? Color.parseColor(dark ? "#86EFAC" : "#166534") : secondaryText
                );
                detail.setPadding(0, dp(5), 0, 0);
                row.addView(detail);
                LinearLayout.LayoutParams params = cardParams();
                params.bottomMargin = dp(9);
                devices.addView(row, params);
            }
        }));
        return wrap(content);
    }

    private View settingsScreen() {
        LinearLayout content = screenContent();
        addScreenTitle(content, "Settings", "Connection and quick clipboard actions.");

        content.addView(sectionLabel("Connection"));
        LinearLayout connection = card();
        connection.addView(settingRow("This device", HubConfig.deviceName(this)));
        connection.addView(divider());
        connection.addView(settingRow("Sync server", HubConfig.hubURL(this)));
        connection.addView(divider());
        connection.addView(settingRow("Recent clips", "20 items · 24 hours"));
        content.addView(connection, cardParams());

        Button edit = actionButton("Edit connection", true);
        edit.setOnClickListener(view -> startActivity(new Intent(this, SettingsActivity.class)));
        content.addView(edit, spaced(dp(12)));

        content.addView(sectionLabel("Quick Settings"));
        LinearLayout quick = card();
        quick.addView(text(
                "Add Send Clipboard to Android's Quick Settings for a one-tap, user-initiated clipboard send.",
                14,
                secondaryText
        ));
        Button tile = actionButton("Add Send Clipboard tile", false);
        tile.setOnClickListener(view -> requestTile());
        quick.addView(tile, spaced(dp(12)));
        content.addView(quick, cardParams());

        content.addView(sectionLabel("About"));
        String version = "";
        try {
            version = getPackageManager().getPackageInfo(getPackageName(), 0).versionName;
        } catch (PackageManager.NameNotFoundException ignored) {
        }
        LinearLayout about = card();
        about.addView(settingRow("Version", version));
        content.addView(about, cardParams());
        return wrap(content);
    }

    private void performPrivacyWipe() {
        privacyWipeRequested = false;
        ClipboardManager clipboard = (ClipboardManager) getSystemService(CLIPBOARD_SERVICE);
        clipboard.clearPrimaryClip();
        HubConfig.setLastSequence(this, 0L);
        LocalClipHistory.clear(this);
        Log.i("ClipboardPrivacy", "Primary clipboard cleared without reading it");
        finish();
    }

    private void performDebugConfiguration() {
        String hubURL = getIntent().getStringExtra("hub_url");
        String deviceName = getIntent().getStringExtra("device_name");
        if (hubURL == null || deviceName == null
                || !(hubURL.startsWith("http://") || hubURL.startsWith("https://"))
                || deviceName.isBlank()) {
            Log.e("TailboardConfig", "Debug configuration arguments are incomplete");
            finish();
            return;
        }
        HubConfig.save(this, hubURL, deviceName);
        ClipboardSyncService.restart(this);
        Log.i("TailboardConfig", "Local configuration saved");
        finish();
    }

    @Override
    @SuppressLint("UnspecifiedRegisterReceiverFlag")
    protected void onStart() {
        super.onStart();
        IntentFilter filter = new IntentFilter(ClipboardSyncService.ACTION_STATUS);
        if (Build.VERSION.SDK_INT >= 33) {
            registerReceiver(statusReceiver, filter, RECEIVER_NOT_EXPORTED);
        } else {
            registerReceiver(statusReceiver, filter);
        }
    }

    @Override
    protected void onStop() {
        unregisterReceiver(statusReceiver);
        super.onStop();
    }

    private void sendClipboard() {
        ClipboardManager clipboard = (ClipboardManager) getSystemService(CLIPBOARD_SERVICE);
        ClipData clip = clipboard.getPrimaryClip();
        if (clip == null || clip.getItemCount() == 0) {
            Toast.makeText(this, "Clipboard is empty", Toast.LENGTH_SHORT).show();
            return;
        }
        CharSequence value = clip.getItemAt(0).coerceToText(this);
        if (value == null || value.length() == 0) {
            Toast.makeText(this, "Only text is supported for now", Toast.LENGTH_SHORT).show();
            return;
        }
        ClipboardSyncService.send(this, value.toString());
    }

    private void receiveClipboard() {
        client.getCurrent(this, (clip, error) -> handler.post(() -> {
            if (clip == null) {
                Toast.makeText(this, error, Toast.LENGTH_LONG).show();
                return;
            }
            LocalClipHistory.add(this, clip);
            copyClip(clip);
        }));
    }

    private void copyClip(HubClient.Clip clip) {
        ClipboardManager clipboard = (ClipboardManager) getSystemService(CLIPBOARD_SERVICE);
        clipboard.setPrimaryClip(ClipData.newPlainText("Tailboard", clip.content));
        Toast.makeText(this, "Copied from " + friendlySource(clip.source), Toast.LENGTH_SHORT).show();
    }

    static String plainText(HubClient.Clip clip) {
        return clip.content;
    }

    private static String clipPreview(HubClient.Clip clip) {
        String value = plainText(clip);
        value = value.trim().replaceAll("\\s+", " ");
        if (value.isEmpty()) return "Empty text";
        return value.length() > 240 ? value.substring(0, 240) + "…" : value;
    }

    private void requestTile() {
        if (Build.VERSION.SDK_INT < 33) {
            Toast.makeText(this, "Edit Quick Settings and drag in Send Clipboard", Toast.LENGTH_LONG).show();
            return;
        }
        StatusBarManager manager = getSystemService(StatusBarManager.class);
        manager.requestAddTileService(
                new ComponentName(this, ClipboardTileService.class),
                "Send Clipboard",
                Icon.createWithResource(this, R.drawable.ic_tailboard_mono),
                getMainExecutor(),
                result -> Toast.makeText(this,
                        result == StatusBarManager.TILE_ADD_REQUEST_RESULT_TILE_ADDED
                                ? "Send Clipboard tile added"
                                : "Open Quick Settings edit mode if the tile was already added",
                        Toast.LENGTH_LONG).show());
    }

    private void setStatus(String status, boolean connected) {
        statusView.setText(status);
        int textColor = connected
                ? Color.parseColor(dark ? "#86EFAC" : "#166534")
                : Color.parseColor(dark ? "#FCD34D" : "#B45309");
        int background = connected
                ? Color.parseColor(dark ? "#153621" : "#F0FDF4")
                : Color.parseColor(dark ? "#3A2A12" : "#FFF7ED");
        statusView.setTextColor(textColor);
        statusView.setBackground(rounded(background, 10));
    }

    private ScrollView wrap(LinearLayout content) {
        ScrollView scroll = new ScrollView(this);
        scroll.setFillViewport(true);
        scroll.setClipToPadding(false);
        scroll.addView(content);
        return scroll;
    }

    private LinearLayout screenContent() {
        LinearLayout content = new LinearLayout(this);
        content.setOrientation(LinearLayout.VERTICAL);
        content.setPadding(dp(20), dp(16), dp(20), dp(34));
        return content;
    }

    private void addScreenTitle(LinearLayout content, String title, String subtitle) {
        TextView heading = text(title, 26, primaryText);
        heading.setTypeface(Typeface.DEFAULT, Typeface.BOLD);
        content.addView(heading);
        TextView description = text(subtitle, 14, secondaryText);
        description.setPadding(0, dp(5), 0, dp(18));
        content.addView(description);
    }

    private LinearLayout card() {
        LinearLayout result = new LinearLayout(this);
        result.setOrientation(LinearLayout.VERTICAL);
        result.setPadding(dp(17), dp(16), dp(17), dp(16));
        result.setBackground(rounded(card, 16));
        result.setElevation(dp(1));
        return result;
    }

    private TextView settingRow(String label, String value) {
        TextView row = text(label + "\n" + value, 15, primaryText);
        row.setLineSpacing(0, 1.12f);
        return row;
    }

    private View divider() {
        View line = new View(this);
        line.setBackgroundColor(separator);
        LinearLayout.LayoutParams params = new LinearLayout.LayoutParams(
                LinearLayout.LayoutParams.MATCH_PARENT,
                dp(1)
        );
        params.topMargin = dp(13);
        params.bottomMargin = dp(13);
        line.setLayoutParams(params);
        return line;
    }

    private TextView sectionLabel(String value) {
        TextView label = text(value, 14, primaryText);
        label.setTypeface(Typeface.DEFAULT, Typeface.BOLD);
        label.setPadding(0, dp(22), 0, dp(9));
        return label;
    }

    private TextView caption(String value) {
        TextView label = text(value, 11, secondaryText);
        label.setTypeface(Typeface.DEFAULT, Typeface.BOLD);
        label.setLetterSpacing(0.08f);
        return label;
    }

    private TextView secondaryMessage(String value) {
        TextView message = text(value, 14, secondaryText);
        message.setPadding(dp(4), dp(10), dp(4), dp(10));
        return message;
    }

    private Button actionButton(String title, boolean prominent) {
        Button button = new Button(this);
        button.setText(title);
        button.setAllCaps(false);
        button.setTextSize(14);
        button.setTypeface(Typeface.DEFAULT, Typeface.BOLD);
        button.setTextColor(prominent ? Color.WHITE : accent);
        button.setBackground(rounded(prominent ? Color.parseColor("#2563EB") : withAlpha(accent, 24), 13));
        button.setPadding(dp(12), 0, dp(12), 0);
        button.setStateListAnimator(null);
        return button;
    }

    private TextView text(String value, int size, int color) {
        TextView view = new TextView(this);
        view.setText(value);
        view.setTextSize(size);
        view.setTextColor(color);
        view.setFontFeatureSettings("kern");
        return view;
    }

    private GradientDrawable rounded(int color, int radius) {
        GradientDrawable drawable = new GradientDrawable();
        drawable.setColor(color);
        drawable.setCornerRadius(dp(radius));
        return drawable;
    }

    private static int withAlpha(int color, int alpha) {
        return Color.argb(alpha, Color.red(color), Color.green(color), Color.blue(color));
    }

    private static String friendlySource(String source) {
        if (source == null || source.isBlank()) return "another device";
        String lower = source.toLowerCase(Locale.ROOT);
        if (lower.equals("mb") || lower.equals("mac") || lower.contains("macbook")) return "MacBook";
        if (lower.contains("iphone")) return "iPhone";
        if (lower.contains("pixel")) return "Pixel";
        return source.substring(0, 1).toUpperCase(Locale.ROOT) + source.substring(1);
    }

    private static String friendlyPlatform(String platform) {
        if (platform == null) return "Device";
        switch (platform.toLowerCase(Locale.ROOT)) {
            case "darwin": return "Mac";
            case "ios": return "iPhone";
            case "android": return "Android";
            case "windows": return "Windows";
            case "linux": return "Linux";
            default: return platform.isBlank() ? "Device" : platform;
        }
    }

    private LinearLayout.LayoutParams matchWrap() {
        return new LinearLayout.LayoutParams(
                LinearLayout.LayoutParams.MATCH_PARENT,
                LinearLayout.LayoutParams.WRAP_CONTENT
        );
    }

    private LinearLayout.LayoutParams cardParams() {
        return matchWrap();
    }

    private LinearLayout.LayoutParams spaced(int top) {
        LinearLayout.LayoutParams params = matchWrap();
        params.topMargin = top;
        return params;
    }

    private int dp(int value) {
        return Math.round(value * getResources().getDisplayMetrics().density);
    }
}
