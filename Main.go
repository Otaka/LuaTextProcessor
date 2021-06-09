package main

import (
	"fmt"
	"os"
)

var filesToProcess []string
var outputFilePath = "console"
var luaFiles []string

func log(message ...interface{}) {
	_, _ = fmt.Fprintln(os.Stderr, message...)
}

func fail(message ...interface{}) {
	log(message...)
	os.Exit(1)
}

func checkFileExists(filePath string) bool {
	_, err := os.Stat(filePath)
	return err == nil
}

func checkCommandLineArgExists(argIndex int, errorMessage string) {
	if argIndex >= len(os.Args) {
		fail(errorMessage)
	}
}

func parseCommandLine() {
	commandLineArgs := os.Args
	commandLineArgsCount := len(commandLineArgs)
	if commandLineArgsCount == 1 {
		printHelp(false)
		os.Exit(1)
	}

	for i := 1; i < commandLineArgsCount; i++ {
		arg := commandLineArgs[i]
		if arg == "-h" || arg == "--help" {
			printHelp(true)
		} else if arg == "-l" {
			i++
			checkCommandLineArgExists(i, "You should provide lua file after -l")
			libraryFilePath := commandLineArgs[i]
			if !checkFileExists(libraryFilePath) {
				fail("Provided lua file", libraryFilePath, "does not exists")
			}

			luaFiles = append(luaFiles, libraryFilePath)
		} else if arg == "-f" {
			i++
			checkCommandLineArgExists(i, "You should provide input file after -i")
			inputFilePath := commandLineArgs[i]
			if !checkFileExists(inputFilePath) {
				fail("Provided input file", inputFilePath, "does not exists")
			}

			filesToProcess = append(filesToProcess, inputFilePath)
		} else if arg == "-o" {
			i++
			checkCommandLineArgExists(i, "You should provide output file path after -o")
			outputFilePath = commandLineArgs[i]
		} else {
			fail("Unknown command line flag", arg)
		}
	}
}

func printHelp(showFullHelp bool) {
	fmt.Println("Lua Text Preprocessor is a tool that can transform text with lua")
	fmt.Println("Usage:")
	fmt.Println("\tluatp -f sourcefile")
	if showFullHelp {
		fmt.Println("Command line flags are:")
		fmt.Println("\t-f\t\t\tfile to process")
		fmt.Println("\t-o\t\t\toutput file. If 'console' - output will be redirected to console. Default - 'console'")
		fmt.Println("\t-i\t\t\tfile that should be processed before processing main file")
		fmt.Println("\t-h, --help\tShow help")
	}

	fmt.Println()
	os.Exit(0)
}

func main() {
	parseCommandLine()
	if len(filesToProcess) == 0 {
		fail("Input file does not specified. Please provide -f input_file_path arguments")
	}

	processFiles(luaFiles, filesToProcess, outputFilePath)
}
