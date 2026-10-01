package com.arjanflac.tailboard;

import android.app.Activity;
import android.content.res.Configuration;
import android.graphics.Color;
import android.graphics.Typeface;
import android.os.Bundle;
import android.text.InputType;
import android.view.View;
import android.widget.RadioButton;
import android.widget.RadioGroup;
import android.widget.ScrollView;
import android.widget.Button;
import android.widget.EditText;
import android.widget.LinearLayout;
import android.widget.TextView;
import android.widget.Toast;

public final class SettingsActivity extends Activity {
    private EditText server;
    private EditText name;
    private EditText macName;

    @Override protected void onCreate(Bundle savedInstanceState) {
        super.onCreate(savedInstanceState);
        boolean dark = (getResources().getConfiguration().uiMode & Configuration.UI_MODE_NIGHT_MASK)
                == Configuration.UI_MODE_NIGHT_YES;
        int background = Color.parseColor(dark ? "#111111" : "#F3F3F1");
        int primary = Color.parseColor(dark ? "#F5F5F3" : "#1F1F1D");
        int secondary = Color.parseColor(dark ? "#A4A49F" : "#6C6C67");

        LinearLayout root = new LinearLayout(this);
        root.setOrientation(LinearLayout.VERTICAL);
        root.setPadding(dp(20), dp(20), dp(20), dp(20));
        root.setOnApplyWindowInsetsListener((view, insets) -> {
            view.setPadding(dp(20), dp(20) + insets.getSystemWindowInsetTop(),
                    dp(20), dp(20) + insets.getSystemWindowInsetBottom());
            return insets;
        });
        root.setBackgroundColor(background);
        TextView title = label("Default Mac", 28, primary, true);
        root.addView(title);
        TextView help = label("Clipboard sends and receives use only the Mac selected here. Both devices need Tailscale and Tailboard running.", 15, secondary, false);
        help.setPadding(0, dp(8), 0, dp(16));
        root.addView(help);

        RadioGroup saved = new RadioGroup(this);
        for (MacDestination destination : TailboardConfig.savedMacs(this)) {
            RadioButton choice = new RadioButton(this);
            choice.setId(View.generateViewId());
            choice.setText(destination.name + "\n" + destination.url);
            choice.setTextColor(primary);
            saved.addView(choice);
            if (destination.url.equals(TailboardConfig.serverURL(this))) saved.check(choice.getId());
            choice.setOnClickListener(view -> {
                macName.setText(destination.name);
                server.setText(destination.url);
            });
        }
        if (saved.getChildCount() > 0) {
            root.addView(label("Saved Macs", 13, secondary, true));
            root.addView(saved);
        }
        MacDestination current = TailboardConfig.destination(this);
        root.addView(label("Mac name", 13, secondary, true), spaced(16));
        macName = field(current.name, primary);
        macName.setHint("MacBook");
        macName.setId(R.id.mac_name);
        root.addView(macName);
        root.addView(label("Tailscale address", 13, secondary, true), spaced(12));
        server = field(current.configured() ? current.url : "", primary);
        server.setHint("macbook or 100.x.x.x");
        server.setInputType(InputType.TYPE_CLASS_TEXT | InputType.TYPE_TEXT_VARIATION_URI);
        server.setId(R.id.mac_address);
        root.addView(server);
        Button add = new Button(this);
        add.setText("Add another Mac");
        add.setAllCaps(false);
        add.setOnClickListener(view -> {
            saved.clearCheck();
            macName.setText("");
            server.setText("");
            macName.requestFocus();
        });
        root.addView(add, spaced(8));
        TextView phoneLabel = label("Phone name", 13, secondary, true);
        phoneLabel.setPadding(0, dp(16), 0, 0);
        root.addView(phoneLabel);
        name = field(TailboardConfig.deviceName(this), primary);
        name.setId(R.id.phone_name);
        root.addView(name);

        Button save = new Button(this);
        save.setText("Save as default Mac");
        save.setAllCaps(false);
        save.setTextSize(16);
        LinearLayout.LayoutParams params = new LinearLayout.LayoutParams(-1, -2);
        params.topMargin = dp(16);
        root.addView(save, params);
        save.setOnClickListener(view -> save());
        ScrollView scroll = new ScrollView(this);
        scroll.setFillViewport(true);
        scroll.addView(root);
        setContentView(scroll);
    }

    private void save() {
        String url = server.getText().toString().trim();
        String phone = name.getText().toString().trim();
        String mac = macName.getText().toString().trim();
        if (phone.isEmpty() || mac.isEmpty()) {
            Toast.makeText(this, "Enter a Mac name and phone name", Toast.LENGTH_LONG).show();
            return;
        }
        try {
            TailboardConfig.save(this, url, mac, phone);
        } catch (IllegalArgumentException error) {
            server.setError(error.getMessage());
            return;
        }
        ClipboardSyncService.restart(this);
        finish();
    }

    private EditText field(String value, int color) {
        EditText field = new EditText(this);
        field.setText(value);
        field.setTextColor(color);
        field.setSingleLine(true);
        field.setTextSize(16);
        field.setMinHeight(dp(48));
        return field;
    }

    private TextView label(String value, int size, int color, boolean bold) {
        TextView view = new TextView(this);
        view.setText(value);
        view.setTextSize(size);
        view.setIncludeFontPadding(false);
        view.setTextColor(color);
        if (bold) view.setTypeface(Typeface.DEFAULT, Typeface.BOLD);
        return view;
    }

    private LinearLayout.LayoutParams spaced(int top) {
        LinearLayout.LayoutParams params = new LinearLayout.LayoutParams(-1, -2);
        params.topMargin = dp(top);
        return params;
    }

    private int dp(int value) {
        return Math.round(value * getResources().getDisplayMetrics().density);
    }
}
