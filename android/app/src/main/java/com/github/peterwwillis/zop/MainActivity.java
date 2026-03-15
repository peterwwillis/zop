package com.github.peterwwillis.zop;

import android.content.SharedPreferences;
import android.os.Bundle;
import android.os.Handler;
import android.os.Looper;
import android.view.Menu;
import android.view.MenuItem;
import android.widget.Button;
import android.widget.EditText;
import android.widget.LinearLayout;
import android.widget.ScrollView;
import android.widget.TextView;
import android.widget.Toast;

import androidx.appcompat.app.AlertDialog;
import androidx.appcompat.app.AppCompatActivity;

import java.util.concurrent.ExecutorService;
import java.util.concurrent.Executors;

import zoplib.Zoplib;

/**
 * MainActivity presents a simple chat interface that sends prompts to an
 * OpenAI-compatible AI provider using the Go zoplib library (built by
 * gomobile bind from internal/zoplib).
 *
 * API credentials and model selection are stored in SharedPreferences and
 * edited via a settings dialog accessible from the action bar.
 */
public class MainActivity extends AppCompatActivity {

    private static final String PREFS_NAME = "zop_settings";
    private static final String PREF_API_KEY = "api_key";
    private static final String PREF_BASE_URL = "base_url";
    private static final String PREF_MODEL = "model";
    private static final String PREF_SYSTEM_PROMPT = "system_prompt";
    private static final String DEFAULT_MODEL = "gpt-4o";

    private ScrollView scrollView;
    private TextView outputView;
    private EditText inputView;
    private Button sendButton;

    /** Accumulates the full conversation transcript displayed in outputView. */
    private final StringBuilder transcript = new StringBuilder();

    /** Single-thread executor keeps API calls off the main thread. */
    private final ExecutorService executor = Executors.newSingleThreadExecutor();
    private final Handler mainHandler = new Handler(Looper.getMainLooper());

    @Override
    protected void onCreate(Bundle savedInstanceState) {
        super.onCreate(savedInstanceState);
        setContentView(R.layout.activity_main);

        scrollView = findViewById(R.id.scrollView);
        outputView = findViewById(R.id.outputView);
        inputView  = findViewById(R.id.inputView);
        sendButton = findViewById(R.id.sendButton);

        sendButton.setOnClickListener(v -> sendQuery());

        // Allow sending with the keyboard's action button.
        inputView.setOnEditorActionListener((v, actionId, event) -> {
            sendQuery();
            return true;
        });
    }

    @Override
    protected void onDestroy() {
        super.onDestroy();
        executor.shutdown();
    }

    // -------------------------------------------------------------------------
    // Sending a query
    // -------------------------------------------------------------------------

    private void sendQuery() {
        String prompt = inputView.getText().toString().trim();
        if (prompt.isEmpty()) {
            Toast.makeText(this, R.string.msg_empty_prompt, Toast.LENGTH_SHORT).show();
            return;
        }

        SharedPreferences prefs = getSharedPreferences(PREFS_NAME, MODE_PRIVATE);
        String apiKey      = prefs.getString(PREF_API_KEY, "");
        String model       = prefs.getString(PREF_MODEL, DEFAULT_MODEL);
        String baseURL     = prefs.getString(PREF_BASE_URL, "");
        String systemPrompt = prefs.getString(PREF_SYSTEM_PROMPT, "");

        if (apiKey.isEmpty()) {
            Toast.makeText(this, R.string.msg_no_api_key, Toast.LENGTH_LONG).show();
            showSettingsDialog();
            return;
        }

        appendLine(getString(R.string.label_you) + ": " + prompt);
        inputView.setText("");
        setSendEnabled(false);

        executor.execute(() -> {
            String result;
            boolean isError = false;
            try {
                result = Zoplib.query(apiKey, baseURL, model, systemPrompt, prompt);
            } catch (Exception e) {
                result = e.getMessage();
                isError = true;
            }

            final String text = result;
            final String label = isError
                    ? getString(R.string.label_error)
                    : getString(R.string.label_zop);

            mainHandler.post(() -> {
                appendLine(label + ": " + text);
                setSendEnabled(true);
            });
        });
    }

    /** Appends a line to the conversation transcript and scrolls to the bottom. */
    private void appendLine(String line) {
        if (transcript.length() > 0) {
            transcript.append("\n\n");
        }
        transcript.append(line);
        outputView.setText(transcript.toString());
        scrollView.post(() -> scrollView.fullScroll(ScrollView.FOCUS_DOWN));
    }

    private void setSendEnabled(boolean enabled) {
        sendButton.setEnabled(enabled);
    }

    // -------------------------------------------------------------------------
    // Settings dialog
    // -------------------------------------------------------------------------

    @Override
    public boolean onCreateOptionsMenu(Menu menu) {
        menu.add(0, Menu.FIRST, 0, R.string.menu_settings);
        return true;
    }

    @Override
    public boolean onOptionsItemSelected(MenuItem item) {
        if (item.getItemId() == Menu.FIRST) {
            showSettingsDialog();
            return true;
        }
        return super.onOptionsItemSelected(item);
    }

    private void showSettingsDialog() {
        SharedPreferences prefs = getSharedPreferences(PREFS_NAME, MODE_PRIVATE);

        // Build the dialog contents programmatically to avoid extra layout files.
        LinearLayout layout = new LinearLayout(this);
        layout.setOrientation(LinearLayout.VERTICAL);
        int padding = dpToPx(16);
        layout.setPadding(padding, padding, padding, padding);

        EditText apiKeyInput = makeEditText(prefs.getString(PREF_API_KEY, ""),
                getString(R.string.hint_api_key));
        EditText baseURLInput = makeEditText(prefs.getString(PREF_BASE_URL, ""),
                getString(R.string.hint_base_url));
        EditText modelInput = makeEditText(prefs.getString(PREF_MODEL, DEFAULT_MODEL),
                getString(R.string.hint_model));
        EditText systemPromptInput = makeEditText(prefs.getString(PREF_SYSTEM_PROMPT, ""),
                getString(R.string.hint_system_prompt));

        layout.addView(apiKeyInput);
        layout.addView(baseURLInput);
        layout.addView(modelInput);
        layout.addView(systemPromptInput);

        new AlertDialog.Builder(this)
                .setTitle(R.string.settings_title)
                .setView(layout)
                .setPositiveButton(R.string.save, (dialog, which) -> {
                    prefs.edit()
                            .putString(PREF_API_KEY, apiKeyInput.getText().toString().trim())
                            .putString(PREF_BASE_URL, baseURLInput.getText().toString().trim())
                            .putString(PREF_MODEL, modelInput.getText().toString().trim())
                            .putString(PREF_SYSTEM_PROMPT, systemPromptInput.getText().toString().trim())
                            .apply();
                })
                .setNegativeButton(R.string.cancel, null)
                .show();
    }

    private EditText makeEditText(String value, String hint) {
        EditText et = new EditText(this);
        et.setHint(hint);
        et.setText(value);
        LinearLayout.LayoutParams lp = new LinearLayout.LayoutParams(
                LinearLayout.LayoutParams.MATCH_PARENT,
                LinearLayout.LayoutParams.WRAP_CONTENT);
        lp.bottomMargin = dpToPx(8);
        et.setLayoutParams(lp);
        return et;
    }

    private int dpToPx(int dp) {
        float density = getResources().getDisplayMetrics().density;
        return Math.round(dp * density);
    }
}
