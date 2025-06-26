## Feature Idea: "Update" Mode for Detecting Source Text Changes

### 1. The Problem

The current "Quick" mode is effective for finding and translating entirely new texts (where the target cell is empty). However, it does not handle cases where an existing source text is modified—for example, to fix a typo or add more detail. The tool will not re-translate these updated texts because the target cell is already filled from a previous run.

### 2. The Solution: The "Fingerprint" Method

To detect changes in source text, we can create a unique "fingerprint" for each text string using a fast hashing algorithm (like MD5 or SHA256). By comparing the fingerprint of the current source text to the one from the last translation, we can instantly tell if it has changed.

### 3. Storing the Fingerprints

Since modifying the translated `.xlsx` file might cause issues with TIA Portal imports, the recommended approach is to store the fingerprints in a separate, standalone file. This keeps the primary spreadsheet clean.

*   **Companion File:** Create a simple key-value file (e.g., `translation_hashes.json`) alongside the translated Excel file.
*   **Structure:** This file would map a unique row identifier to the source text's hash.
    *   **Unique ID:** A reliable ID can be created by combining the values from the first few columns of a row (e.g., `Text list`, `ID`, etc.), as these should uniquely identify a text entry.
    *   **Example `translation_hashes.json`:**
        ```json
        {
          "Alarms_101": "e5d9f678a4b6c3b0...",
          "HMI_Messages_204": "a1b2c3d4e5f6g7h8..."
        }
        ```

### 4. Proposed "Update" Mode Logic

A new "Update" mode would be added to the TUI. When selected, it would perform the following steps for each row:

1.  **Generate Unique ID:** Create the unique ID for the current row.
2.  **Calculate New Hash:** Calculate the hash of the current source text.
3.  **Compare Hashes:**
    *   Look up the **old hash** in the `translation_hashes.json` file using the row's ID.
    *   If the **new hash** does not match the **old hash** (or if no old hash exists), the text has changed. **Action: Re-translate the text.**
    *   If the hashes are identical, the text is unchanged. **Action: Skip.**
4.  **Update Stored Hash:** After a successful translation, save the **new hash** to the `.json` file to ensure it's up-to-date for the next run.

### 5. Benefits

*   **Accurate:** Catches every source text modification, no matter how small.
*   **Clean:** Does not alter the structure of the primary `.xlsx` file, ensuring compatibility with TIA Portal.
*   **Fast:** The time cost of hashing is negligible and will not add any noticeable delay to the process.
