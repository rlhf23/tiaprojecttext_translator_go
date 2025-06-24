
import fs from 'fs'; 
import readline from 'readline';

// List all .xlsx files in the directory that don't start with "translated"
let files = fs.readdirSync('.').filter(file => file.endsWith('.xlsx') && !file.startsWith('translated'));
files.forEach((file, index) => {
    console.log(`${index + 1}: ${file}`);
});

// Create readline interface
const rl = readline.createInterface({
    input: process.stdin,
    output: process.stdout
});

// Load the spreadsheet
let workbook;
let xlDataArray;
let headers;
let targetLanguageColumn;
let sourceLanguageColumn;

// row count function, really simple, will guesstimate and overshoot a lot
async function countRowsInFile(fileName) {
    const fileStream = fs.createReadStream(fileName);

    const rlFile = readline.createInterface({
        input: fileStream,
        crlfDelay: Infinity
    });

    let rowCount = 0;
    for await (const line of rlFile) {
        if (line.trim() !== '') {
            rowCount++;
        }
    }

    return rowCount;
}

// Ask the user to select a file
rl.question('Enter the number of the file you want to translate: ', async (answer) => {
    let fileIndex = parseInt(answer);
    if (fileIndex >= 1 && fileIndex <= files.length) {
        let fileName = files[fileIndex - 1];
        console.log(`You selected ${fileName}`);

        // Calculate row count
        const rowCount = await countRowsInFile(fileName);

        console.log(`The file contains somehwere around up to maybe ${rowCount} rows, or far fewer idke.`);

        // Load and translate the selected file...
        try {
            workbook = XLSX.readFile(fileName);
            let sheet_name_list = workbook.SheetNames;
            xlDataArray = XLSX.utils.sheet_to_json(workbook.Sheets[sheet_name_list[0]], { header: 1, raw: false });
            headers = xlDataArray[0];

            // List the headers
            headers.forEach((header, index) => {
                if (!header.toLowerCase().startsWith("ref") && index >= 4) {
                    console.log(`${index + 1}: ${header}`);
                }
            });

            // Ask the user to select the source language column
            rl.question(`Enter the number of the source language column ([6]: ${headers[6 - 1]}): `, (answer) => {
                let sourceIndex = answer ? parseInt(answer) : 6;
                if (sourceIndex >= 1 && sourceIndex <= headers.length) {
                    sourceLanguageColumn = headers[sourceIndex - 1];
                    console.log(`Source language column: ${sourceLanguageColumn}`);

                    // Ask the user to select the target language column
                    rl.question(`Enter the number of the target language column ([7]: ${headers[7 - 1]}): `, (answer) => {
                        let targetIndex = answer ? parseInt(answer) : 7;
                        if (targetIndex >= 1 && targetIndex <= headers.length) {
                            targetLanguageColumn = headers[targetIndex - 1];
                            console.log(`Target language column: ${targetLanguageColumn}`);

                            iterateAndTranslate(sourceLanguageColumn, targetLanguageColumn, fileName);
                            rl.close();
                        } else {
                            console.error('Invalid selection for target language column');
                            rl.close();
                        }
                    });
                } else {
                    console.error('Invalid selection for source language column');
                    rl.close();
                }
            });

        } catch (error) {
            console.error(`Error reading Excel file: ${error.message}`);
            process.exit(1); // Exit the script if the Excel file cannot be read
        }
    } else {
        console.error('Invalid selection');
        rl.close();
    }
});

import XLSX from 'xlsx';
import { Configuration, OpenAIApi } from 'openai';

// Initialize the OpenAI client
const openai = new OpenAI({
    apiKey: process.env.OPENAI_API_KEY
});

// Translation function using OpenAI's chat model
async function translateText(text, sourceLanguageCode, targetLanguageCode) {
    try {
        const response = await Promise.race([
            openai.createChatCompletion({
                model: "gpt-4o-mini",
                messages: [
                    {
                        role: "system",
                        content: `You will be provided with a sentence in ${sourceLanguageCode}, and your task is to translate it into ${targetLanguageCode}. These 
                        are messages concerning industrial machines. Right means the direction right. AC means AC motor.`
                    },
                    {
                        role: "user",
                        content: `${text}`
                    }
                ],
                temperature: 0,
                max_tokens: 60
            }),
            new Promise((_, reject) => setTimeout(() => reject(new Error('Request timed out')), 5000))  // 5 seconds timeout
        ]);

        // Extract the translated text from the response
        let translatedText = response.data.choices[0].message.content.trim();
        return translatedText;
    } catch (error) {
        if (error.response && error.response.status === 429) {
            console.log('Rate limit exceeded');
            console.log('Retry after', error.response.headers['retry-after'], 'seconds');
        } else if (error.message === 'Request timed out') {
            console.error('The translation request timed out.');
            return null;
        } else {
            console.error(`Error during translation: ${error.message}`);
            return null;
        }
    }
}

// Iterate over the rows of the Excel file
async function iterateAndTranslate(sourceLanguageColumn, targetLanguageColumn, fileName) {
    let targetLanguageIndex = headers.indexOf(targetLanguageColumn);
    let previousText = null;
    let previousTranslation = null;
    let sourceLangIndex = headers.indexOf(sourceLanguageColumn);

    for (let i = 1; i < xlDataArray.length; i++) {
        try {

            if (xlDataArray[i][sourceLangIndex]) {
                let text = xlDataArray[i][sourceLangIndex].trim();
                // If the current row text is the same as the previous row text, skip translation and copy the previous translation
                if (previousText === text) {
                    xlDataArray[i][targetLanguageIndex] = previousTranslation;
                    console.log(`Row ${i} copied from previous row: ${previousTranslation}`);
                }
                // If not, check for empty strings, pure number strings, or strings that contain only special characters
                else if (
                    // Check if the text is not an empty string
                    text.length > 0 &&

                    // Check if the text is not a numeric value
                    isNaN(text) &&

                    // Check if the text contains at least five word characters
                    /\w{4,}/.test(text)
                ) {
                    // Check if the text contains a hashtag
                    if (text.includes("#")) {
                        let textParts = text.split("#");
                        let previousTextParts = previousText ? previousText.split("#") : null;

                        // Check if the first part of the text matches the first part of the previous text
                        if (previousTextParts && textParts[0].trim() === previousTextParts[0].trim()) {
                            let previousTranslationParts = previousTranslation.split("#");

                            // If there's nothing after the hashtag but numbers, or it matches the specific pattern, then don't translate
                            let translatedSecondPart =
                                isNaN(textParts[1].trim()) &&
                                    !/^A\d+-\d+:$/.test(textParts[1].trim()) ?
                                    await translateText(textParts[1], sourceLanguageColumn, targetLanguageColumn) :
                                    textParts[1];

                            xlDataArray[i][targetLanguageIndex] = previousTranslationParts[0].trim() + " #" + translatedSecondPart;
                            console.log(`Row ${i} partially copied from previous row: ${xlDataArray[i][targetLanguageIndex]}`);
                            continue;
                        }
                    }
                    // Your translation logic...
                    let translation = await translateText(text, sourceLanguageColumn, targetLanguageColumn);

                    if (translation) {
                        xlDataArray[i][targetLanguageIndex] = translation;
                        console.log(`Row ${i} translated successfully: ${translation}`);
                        // Update the previousText and previousTranslation for the next iteration
                        previousText = text;
                        previousTranslation = translation;
                    } else {
                        console.log(`Translation failed for row ${i}: ${JSON.stringify(xlDataArray[i])}`);
                    }
                } else {
                    console.log(`1: Skipped row ${i + 1} due to invalid data: ${text}`);
                    xlDataArray[i][targetLanguageIndex] = text;  // Copy the original text to the target language column
                }
            } else {
                console.log(`2: Skipped row ${i + 1} due to invalid data: ${text}`);
            }
        } catch (error) {
            console.error(`Error on row ${i + 1}: ${error.message}`);
            continue;  // you can choose to continue or to stop the execution by removing this line

        }
    }

    // Create a new worksheet from the updated xlData
    let updatedWorksheet = XLSX.utils.aoa_to_sheet(xlDataArray);

    // Replace the old worksheet in the workbook
    workbook.Sheets[workbook.SheetNames[0]] = updatedWorksheet;

    // Write to a new Excel file
    try {
        let newFileName = 'translated_' + fileName;
        XLSX.writeFile(workbook, newFileName);
        console.log(`Written out: ${newFileName}`);
    } catch (error) {
        console.error(`Error writing to Excel file: ${error.message}`);
    }
}
