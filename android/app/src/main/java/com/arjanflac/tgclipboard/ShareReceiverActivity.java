package com.arjanflac.tgclipboard;

import android.app.Activity;
import android.app.AlertDialog;
import android.content.ClipData;
import android.content.Intent;
import android.net.Uri;
import android.os.Build;
import android.os.Bundle;
import android.widget.Toast;

import java.util.ArrayList;
import java.util.List;

public final class ShareReceiverActivity extends Activity {
    @Override
    protected void onCreate(Bundle savedInstanceState) {
        super.onCreate(savedInstanceState);
        Intent share = getIntent();
        ArrayList<Uri> uris = extractURIs(share);
        CharSequence text = share.getCharSequenceExtra(Intent.EXTRA_TEXT);

        if (!uris.isEmpty()) {
            String defaultTarget = HubConfig.defaultTransferDevice(this).trim();
            if (defaultTarget.isEmpty()) {
                chooseDestination(uris);
                return;
            }
            sendFiles(uris, defaultTarget);
        } else if (text != null && text.length() > 0) {
            ClipboardSyncService.send(this, text.toString());
        } else {
            Toast.makeText(this, "Nothing to send", Toast.LENGTH_SHORT).show();
        }
        finish();
    }

    private void chooseDestination(ArrayList<Uri> uris) {
        HubClient client = new HubClient();
        client.listDevices(this, (devices, error) -> runOnUiThread(() -> {
            if (isFinishing()) return;
            if (error != null) {
                Toast.makeText(this, "Can't load devices: " + error, Toast.LENGTH_LONG).show();
                finish();
                return;
            }
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
            if (choices.isEmpty()) {
                Toast.makeText(this, "No file-transfer devices are available", Toast.LENGTH_LONG).show();
                finish();
                return;
            }
            String[] labels = new String[choices.size()];
            for (int index = 0; index < choices.size(); index++) {
                HubClient.Device device = choices.get(index);
                labels[index] = device.name + (device.online ? "" : " — offline");
            }
            new AlertDialog.Builder(this)
                    .setTitle(uris.size() == 1 ? "Send file to" : "Send files to")
                    .setItems(labels, (dialog, index) -> sendFiles(uris, choices.get(index).name))
                    .setNegativeButton("Cancel", (dialog, which) -> finish())
                    .setOnCancelListener(dialog -> finish())
                    .show();
        }));
    }

    private void sendFiles(ArrayList<Uri> uris, String target) {
        Toast.makeText(
                this,
                (uris.size() == 1 ? "Sending file to " : "Sending files to ")
                        + HubConfig.friendlyTransferDevice(target) + "…",
                Toast.LENGTH_SHORT
        ).show();
        ClipboardSyncService.sendFiles(this, uris, target);
        finish();
    }

    @SuppressWarnings("deprecation")
    private ArrayList<Uri> extractURIs(Intent intent) {
        ArrayList<Uri> result = new ArrayList<>();
        if (Intent.ACTION_SEND_MULTIPLE.equals(intent.getAction())) {
            ArrayList<Uri> shared;
            if (Build.VERSION.SDK_INT >= 33) {
                shared = intent.getParcelableArrayListExtra(Intent.EXTRA_STREAM, Uri.class);
            } else {
                shared = intent.getParcelableArrayListExtra(Intent.EXTRA_STREAM);
            }
            if (shared != null) result.addAll(shared);
        } else if (Intent.ACTION_SEND.equals(intent.getAction())) {
            Uri shared;
            if (Build.VERSION.SDK_INT >= 33) {
                shared = intent.getParcelableExtra(Intent.EXTRA_STREAM, Uri.class);
            } else {
                shared = intent.getParcelableExtra(Intent.EXTRA_STREAM);
            }
            if (shared != null) result.add(shared);
        }

        ClipData clipData = intent.getClipData();
        if (clipData != null) {
            for (int index = 0; index < clipData.getItemCount(); index++) {
                Uri uri = clipData.getItemAt(index).getUri();
                if (uri != null && !result.contains(uri)) result.add(uri);
            }
        }
        return result;
    }
}
