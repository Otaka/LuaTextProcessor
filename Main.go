package main

import (
	"fmt"
	"os"
)

const version = "0.2"

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
		if arg == "-v" || arg == "--version" {
			printVersion()
			os.Exit(0)
		} else if arg == "-h" || arg == "--help" {
			printHelp(true)
			os.Exit(0)
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
func printVersion() {
	fmt.Println("luatp " + version)
	fmt.Println("https://github.com/Otaka/LuaTextProcessor")
}

func printHelp(showFullHelp bool) {
	fmt.Println("luatp\nLua Text Preprocessor - tool that can preprocess text with lua scripts")
	fmt.Println("Usage: luatp -f sourcefile")
	if showFullHelp {
		fmt.Println("Command line flags:")
		fmt.Println("\t-f\t\t\t\tfile to process")
		fmt.Println("\t-o\t\t\t\toutput file. If 'console' - output will be redirected to console. Default - 'console'")
		fmt.Println("\t-l\t\t\t\tfile that should be processed before processing main file")
		fmt.Println("\t-h, --help\t\tShow help")
		fmt.Println("\t-v, --version\tShow version")
	}

	fmt.Println()
}

func main() {
	parseCommandLine()
	if len(filesToProcess) == 0 {
		fail("Input file does not specified. Please provide -f input_file_path arguments")
	}

	processFiles(luaFiles, filesToProcess, outputFilePath)
}
