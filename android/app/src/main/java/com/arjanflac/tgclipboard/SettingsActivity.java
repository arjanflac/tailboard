package com.arjanflac.tgclipboard;

import android.app.Activity;
import android.graphics.Color;
import android.os.Bundle;
import android.widget.Button;
import android.widget.ArrayAdapter;
import android.widget.EditText;
import android.widget.LinearLayout;
import android.widget.ScrollView;
import android.widget.Spinner;
import android.widget.TextView;
import android.widget.Toast;

import java.util.ArrayList;
import java.util.List;

public final class SettingsActivity extends Activity {
    private EditText hubField;
    private EditText nameField;
    private Spinner targetField;
    private final List<String> targetValues = new ArrayList<>();
    private final HubClient client = new HubClient();

    @Override
    protected void onCreate(Bundle savedInstanceState) {
        super.onCreate(savedInstanceState);

        ScrollView scroll = new ScrollView(this);
        LinearLayout content = new LinearLayout(this);
        content.setOrientation(LinearLayout.VERTICAL);
        content.setPadding(dp(24), dp(48), dp(24), dp(32));
        scroll.addView(content);

        TextView title = label("Tailboard settings", 28, true);
        content.addView(title);
        TextView explanation = label(
                "These values are already configured. Change them only if you rename a Tailscale device or move the hub.",
                15,
                false);
        explanation.setTextColor(Color.rgb(75, 85, 99));
        explanation.setPadding(0, dp(8), 0, dp(24));
        content.addView(explanation);

        content.addView(section("Sync server"));
        hubField = field(HubConfig.hubURL(this));
        content.addView(hubField);

        content.addView(section("This device"));
        nameField = field(HubConfig.deviceName(this));
        content.addView(nameField);

        content.addView(section("Default file target"));
        targetField = new Spinner(this);
        content.addView(targetField);
        loadTransferDevices();

        TextView targetNote = label(
                "Ask every time shows a device chooser in the share sheet. A saved destination sends immediately.",
                13,
                false
        );
        targetNote.setTextColor(Color.rgb(75, 85, 99));
        targetNote.setPadding(0, dp(6), 0, 0);
        content.addView(targetNote);

        Button save = new Button(this);
        save.setText("Save and reconnect");
        save.setAllCaps(false);
        save.setTextSize(16);
        LinearLayout.LayoutParams buttonParams = new LinearLayout.LayoutParams(
                LinearLayout.LayoutParams.MATCH_PARENT,
                LinearLayout.LayoutParams.WRAP_CONTENT);
        buttonParams.topMargin = dp(24);
        content.addView(save, buttonParams);
        save.setOnClickListener(view -> save());

        setContentView(scroll);
    }

    private void save() {
        String hub = hubField.getText().toString().trim();
        String name = nameField.getText().toString().trim();
        int targetIndex = targetField.getSelectedItemPosition();
        String target = targetIndex >= 0 && targetIndex < targetValues.size()
                ? targetValues.get(targetIndex)
                : "";
        if (!(hub.startsWith("http://") || hub.startsWith("https://"))
                || name.isEmpty()) {
            Toast.makeText(this, "Enter valid connection settings", Toast.LENGTH_LONG).show();
            return;
        }
        HubConfig.save(this, hub, name);
        HubConfig.setDefaultTransferDevice(this, target);
        ClipboardSyncService.restart(this);
        Toast.makeText(this, "Settings saved", Toast.LENGTH_SHORT).show();
        finish();
    }

    private void loadTransferDevices() {
        String current = HubConfig.defaultTransferDevice(this).trim();
        List<String> initialLabels = new ArrayList<>();
        initialLabels.add("Ask every time");
        targetValues.clear();
        targetValues.add("");
        if (!current.isEmpty()) {
            initialLabels.add(HubConfig.friendlyTransferDevice(current) + " — loading…");
            targetValues.add(current);
        }
        setTargetChoices(initialLabels, current.isEmpty() ? 0 : 1);

        client.listDevices(this, (devices, error) -> runOnUiThread(() -> {
            if (isFinishing()) return;
            List<HubClient.Device> choices = new ArrayList<>();
            for (HubClient.Device device : devices) {
                if (!device.deviceID.equals(HubConfig.deviceID(this)) && device.supportsTransfers) {
                    choices.add(device);
                }
            }
            choices.sort((left, right) -> {
                if (left.online != right.online) return left.online ? -1 : 1;
                return left.name.compareToIgnoreCase(right.name);
            });

            List<String> labels = new ArrayList<>();
            labels.add("Ask every time");
            targetValues.clear();
            targetValues.add("");
            int selection = 0;
            for (HubClient.Device device : choices) {
                labels.add(device.name + (device.online ? "" : " — offline"));
                targetValues.add(device.name);
                if (device.name.equalsIgnoreCase(current)) selection = targetValues.size() - 1;
            }
            if (!current.isEmpty() && selection == 0) {
                labels.add(HubConfig.friendlyTransferDevice(current) + " — unavailable");
                targetValues.add(current);
                selection = targetValues.size() - 1;
            }
            setTargetChoices(labels, selection);
        }));
    }

    private void setTargetChoices(List<String> labels, int selection) {
        ArrayAdapter<String> adapter = new ArrayAdapter<>(
                this,
                android.R.layout.simple_spinner_item,
                labels
        );
        adapter.setDropDownViewResource(android.R.layout.simple_spinner_dropdown_item);
        targetField.setAdapter(adapter);
        targetField.setSelection(selection);
    }

    private TextView section(String text) {
        TextView view = label(text, 15, true);
        view.setPadding(0, dp(18), 0, dp(6));
        return view;
    }

    private TextView label(String text, int size, boolean bold) {
        TextView view = new TextView(this);
        view.setText(text);
        view.setTextSize(size);
        view.setTextColor(Color.rgb(17, 24, 39));
        if (bold) view.setTypeface(view.getTypeface(), android.graphics.Typeface.BOLD);
        return view;
    }

    private EditText field(String value) {
        EditText field = new EditText(this);
        field.setText(value);
        field.setSingleLine(true);
        field.setTextSize(16);
        field.setPadding(dp(12), dp(10), dp(12), dp(10));
        field.setLayoutParams(new LinearLayout.LayoutParams(
                LinearLayout.LayoutParams.MATCH_PARENT,
                LinearLayout.LayoutParams.WRAP_CONTENT));
        return field;
    }

    private int dp(int value) {
        return Math.round(value * getResources().getDisplayMetrics().density);
    }
}
