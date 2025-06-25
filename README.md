Short CLI for translating bulk Project Text exports from Tia Portal projects via the Openai api. It goes down line by line, sending texts and getting replies, sometimes tries to optimize by skipping non-text and reusing text stubs. This is just a simple proof of concept, maybe for doing layouts and such, obviously not meant for production.

------

### How to Run

1.  Place the `translator.exe` and your exported `.xlsx` file in the same folder.
2.  Open a Command Prompt or PowerShell in that folder.
3.  Provide your OpenAI API key using one of the methods below.
4.  Run the program by typing `translator.exe`.

### Providing Your OpenAI API Key

The translator needs an API key from OpenAI to function. You can provide it in one of three ways, listed in order of priority:

**1. Environment Variable (Recommended for developers)**

The program will first check for an environment variable named `OPENAI_API_KEY`. You can set this for your current terminal session.

-   In **Command Prompt**:
    ```cmd
    set OPENAI_API_KEY=your-secret-api-key
    ```
-   In **PowerShell**:
    ```powershell
    $env:OPENAI_API_KEY="your-secret-api-key"
    ```

**2. `api-key.txt` File (Easiest method)**

If the environment variable is not set, the program will look for a file named `api-key.txt` in the same directory as `translator.exe`.

1.  Create a new text file named `api-key.txt`.
2.  Paste your secret OpenAI key into the file and save it.

**3. On-Screen Prompt**

If neither of the above methods is used, the program will prompt you to enter your API key directly in the terminal when you run it. The key will be hidden for privacy and is only used for the current session.

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