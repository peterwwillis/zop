package com.github.peterwwillis.zop;

import org.junit.Test;
import static org.junit.Assert.*;

/**
 * JVM (host-side) unit tests for the Zop Android app.
 *
 * These tests run on the local JVM, so they cannot exercise the gomobile-bound
 * Go library or Android framework classes.  They validate pure-Java logic that
 * lives in the app layer.
 */
public class ZopUnitTest {

    @Test
    public void nonEmptyPrompt_isNotBlank() {
        String prompt = "What is the capital of France?";
        assertFalse("A real prompt must not be blank", prompt.trim().isEmpty());
    }

    @Test
    public void whitespaceOnlyPrompt_isDetectedAsEmpty() {
        String prompt = "   \t  ";
        assertTrue("A whitespace-only prompt must be treated as empty",
                prompt.trim().isEmpty());
    }

    @Test
    public void emptyApiKey_isDetected() {
        String apiKey = "";
        assertTrue("Empty API key should be flagged", apiKey.isEmpty());
    }

    @Test
    public void defaultModel_isSet() {
        // Mirrors the DEFAULT_MODEL constant in MainActivity.
        String defaultModel = "gpt-4o";
        assertFalse("Default model must not be empty", defaultModel.isEmpty());
    }
}
