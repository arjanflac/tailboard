package com.arjanflac.tailboard;

import android.app.Activity;
import android.content.res.Configuration;
import android.graphics.Color;
import android.graphics.Typeface;
import android.os.Bundle;
import android.widget.Button;
import android.widget.EditText;
import android.widget.LinearLayout;
import android.widget.TextView;
import android.widget.Toast;

public final class SettingsActivity extends Activity {
    private EditText server;
    private EditText name;

    @Override protected void onCreate(Bundle savedInstanceState) {
        super.onCreate(savedInstanceState);
        boolean dark = (getResources().getConfiguration().uiMode & Configuration.UI_MODE_NIGHT_MASK)
                == Configuration.UI_MODE_NIGHT_YES;
        int background = Color.parseColor(dark ? "#111111" : "#F3F3F1");
        int primary = Color.parseColor(dark ? "#F5F5F3" : "#1F1F1D");
        int secondary = Color.parseColor(dark ? "#A4A49F" : "#6C6C67");

        LinearLayout root = new LinearLayout(this);
        root.setOrientation(LinearLayout.VERTICAL);
        root.setPadding(dp(24), dp(48), dp(24), dp(32));
        root.setBackgroundColor(background);
        TextView title = label("Connection", 28, primary, true);
        root.addView(title);
        TextView help = label("The Mac address must be reachable through Tailscale.", 15, secondary, false);
        help.setPadding(0, dp(8), 0, dp(24));
        root.addView(help);

        root.addView(label("Mac URL", 13, secondary, true));
        server = field(TailboardConfig.serverURL(this), primary);
        root.addView(server);
        TextView phoneLabel = label("Phone name", 13, secondary, true);
        phoneLabel.setPadding(0, dp(20), 0, 0);
        root.addView(phoneLabel);
        name = field(TailboardConfig.deviceName(this), primary);
        root.addView(name);

        Button save = new Button(this);
        save.setText("Save and reconnect");
        save.setAllCaps(false);
        save.setTextSize(16);
        LinearLayout.LayoutParams params = new LinearLayout.LayoutParams(-1, -2);
        params.topMargin = dp(28);
        root.addView(save, params);
        save.setOnClickListener(view -> save());
        setContentView(root);
    }

    private void save() {
        String url = server.getText().toString().trim();
        String phone = name.getText().toString().trim();
        if (!(url.startsWith("http://") || url.startsWith("https://")) || phone.isEmpty()) {
            Toast.makeText(this, "Enter a valid Mac URL and phone name", Toast.LENGTH_LONG).show();
            return;
        }
        TailboardConfig.save(this, url, phone);
        ClipboardSyncService.restart(this);
        finish();
    }

    private EditText field(String value, int color) {
        EditText field = new EditText(this);
        field.setText(value);
        field.setTextColor(color);
        field.setSingleLine(true);
        field.setTextSize(16);
        return field;
    }

    private TextView label(String value, int size, int color, boolean bold) {
        TextView view = new TextView(this);
        view.setText(value);
        view.setTextSize(size);
        view.setTextColor(color);
        if (bold) view.setTypeface(Typeface.DEFAULT, Typeface.BOLD);
        return view;
    }

    private int dp(int value) {
        return Math.round(value * getResources().getDisplayMetrics().density);
    }
}
