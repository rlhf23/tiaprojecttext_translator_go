Rockwell Processing Flow

For each row in Rockwell file:
1. Check **REF:N** patterns:
   - Source has REF, target empty → copy source
   - Both have REF → skip
   - Target has REF → skip
2. Check embedded refs /*...*/:
   - Quick mode + target already has refs → skip
   - Otherwise → split, translate text, preserve refs, reassemble
3. Example:
   Source: "Calibration weight: /*N:6 {#1.Value}*/ /*S:0 {Unit}*/"
   → Split: ["Calibration weight: ", "/*N:6 ...*/", " ", "/*S:0 ...*/", ""]
   → Translate text segments only
   → Result: "Peso de calibración: /*N:6 {#1.Value}*/ /*S:0 {Unit}*/"
