Short CLI for translating bulk Project Text exports from Tia Portal projects via the Openai api. It goes down line by line, sending texts and getting replies, sometimes tries to optimize by skipping non-text and reusing text stubs. This is just a simple proof of concept, maybe for doing layouts and such, obviously not meant for production.

------

To run it, open a Command Prompt or PowerShell, navigate to the directory where you saved the file, and run it from there.

Remember, since it's a command-line application, you will need to set the OPENAI_API_KEY environment variable on the Windows machine before running it. You can do this in Command Prompt with:

```cmd
set OPENAI_API_KEY=your-secret-api-key
```
Or in PowerShell with:

```powershell
$env:OPENAI_API_KEY="your-secret-api-key"
```
After that, you can run the program by simply typing translator.exe.

------

To create a smaller executable for distribution, you can use the following steps.

1.  Build with Linker Flags:
    This command strips debugging information from the binary, significantly reducing its size.

    ```bash
    go build -ldflags="-s -w" -o translator.exe .
    ```

2.  Compress with UPX:
    For even greater size reduction, compress the stripped binary using `upx`.

    ```bash
    upx --best -o translator_compressed.exe translator.exe
    ```