package main

import (
	"bufio"
	"container/list"
	"fmt"
	"github.com/yuin/gopher-lua"
	"io/ioutil"
	"os"
	"strconv"
	"strings"
	"unicode"
)

const luaStartBlockMarker = "<?lua"
const luaEndBlockMarker = "lua?>"

var content string
var contentLength int
var currentPosition = 0
var currentLineNumber = 0
var currentFile string

var charsStack *CharsStack

var markedBlocks = make(map[string]*Token)
var macroMap = make(map[string]MacroStruct)
var generateLineInfoCallback *lua.LFunction

type includePredicate func(rune) bool

type MacroStruct struct {
	name      string
	arguments []string
	variadic  bool
	callback  *lua.LFunction
}

type CharsStack struct {
	chars []rune
}

func (c *CharsStack) push(char rune) {
	c.chars = append(c.chars, char)
}

func (c *CharsStack) pop() rune {
	n := len(c.chars) - 1
	result := c.chars[n]
	c.chars[n] = 0
	c.chars = c.chars[:n]
	return result
}

func (c *CharsStack) peek() rune {
	n := len(c.chars) - 1
	result := c.chars[n]
	return result
}

func (c *CharsStack) length() int {
	return len(c.chars)
}

func (c *CharsStack) isEmpty() bool {
	return len(c.chars) == 0
}

const (
	EOF             = -1
	WHITESPACE      = 1
	SYMBOL          = 2
	LUA_BLOCK_START = 3
	LUA_BLOCK_END   = 4
	LuaBlock        = 5
	SPECIAL         = 6
	UNKNOWN         = 7
	NUMBER          = 8
)

type Token struct {
	tokenType int
	value     string
	lineIndex int
	inputFile string
}

func (token *Token) String() string {
	return fmt.Sprintf("%d-\"%s\"", token.tokenType, token.value)
}

func checkCurrentBufferContainsString(str string) bool {
	var tempStack CharsStack
	matched := true
	for i := 0; i < len(str); i++ {
		c, success := getChar(true)
		if success == false {
			matched = false
			break
		}

		tempStack.push(c)
		if c != rune(str[i]) {
			matched = false
			break
		}
	}
	//return chars back
	for !tempStack.isEmpty() {
		unGetChar(tempStack.pop())
	}

	return matched
}

func eof() bool {
	if !charsStack.isEmpty() {
		return false
	}
	return currentPosition >= contentLength
}

func skipChars(count int) {
	for i := 0; i < count; i++ {
		getChar(true)
	}
}

func readTokenWhilePredicate(tokenType int, predicate includePredicate) *Token {
	var result strings.Builder
	lineNumber := currentLineNumber
	for !eof() {
		c, success := getChar(true)
		if !success {
			break
		}
		if predicate(c) {
			result.WriteRune(c)
		} else {
			unGetChar(c)
			break
		}
	}

	return &Token{tokenType, result.String(), lineNumber, currentFile}
}

func readTokenUntilString(tokenType int, untilString string) *Token {
	startSymbol := rune(untilString[0])
	lineNumber := currentLineNumber
	found := false
	var result strings.Builder
	for !eof() {
		tempChar, _ := getChar(false)
		if startSymbol == tempChar {
			if checkCurrentBufferContainsString(untilString) {
				found = true
				break
			}
		}

		c, success := getChar(true)
		if !success {
			break
		}
		result.WriteRune(c)
	}
	if found {
		return &Token{tokenType, result.String(), lineNumber, currentFile}
	}

	return &Token{UNKNOWN, result.String(), lineNumber, currentFile}
}

func readWhitespaceToken() *Token {
	return readTokenWhilePredicate(WHITESPACE, func(c rune) bool {
		return unicode.IsSpace(c)
	})
}

func readNumberToken() *Token {
	return readTokenWhilePredicate(NUMBER, func(c rune) bool {
		return unicode.IsNumber(c) || c == '.'
	})
}

func readPunctToken() *Token {
	return readTokenWhilePredicate(SPECIAL, func(c rune) bool {
		return unicode.IsPunct(c)
	})
}

func readSymbolToken() *Token {
	return readTokenWhilePredicate(SYMBOL, func(c rune) bool {
		return unicode.IsLetter(c) || unicode.IsNumber(c) || c == '_'
	})
}

func debugPrint(tokens *list.List, currentNode *list.Element) {
	debugEnabled := false
	if debugEnabled {
		for e := tokens.Front(); e != nil; e = e.Next() {
			token := e.Value.(*Token)
			str := strings.Replace(token.value, "\n", " ", -1)
			str = strings.Replace(str, "\r", " ", -1)
			if e == currentNode {
				print("[^" + strconv.Itoa(token.tokenType) + " #" + strconv.Itoa(token.lineIndex) + " " + str + "]")
			} else {
				print("[" + strconv.Itoa(token.tokenType) + " #" + strconv.Itoa(token.lineIndex) + " " + str + "]")
			}
		}
		print("\n")
	}
}

func flushAndClose(writer *bufio.Writer, file *os.File) {
	_ = writer.Flush()
	_ = file.Close()
}

func processFiles(luaFiles []string, filesToProcess []string, outputFilePath string) {

	luaState := lua.NewState()
	registerFunctions(luaState)
	defer luaState.Close()

	var writer *bufio.Writer
	if outputFilePath == "console" {
		writer = bufio.NewWriter(os.Stdout)
	} else {
		myFile, err := os.Create(outputFilePath)
		if err != nil {
			panic(err)
		}
		writer = bufio.NewWriter(myFile)
		defer flushAndClose(writer, myFile)
	}
	//Execute lua files
	for _, file := range luaFiles {
		fileContent := readFile(file)
		if err := luaState.DoString(fileContent); err != nil {
			log("Error while processing lua file:" + file + "\n" + err.Error())
			os.Exit(1)
			//reportErrorAndExit(nil, "")
			//panic(err)
		}
	}

	for _, file := range filesToProcess {
		currentFile = file
		processFile(readFile(file), luaState, writer)
		currentFile = ""
	}
}

func readFile(filePath string) string {
	fileByteContent, err := ioutil.ReadFile(filePath)
	if err != nil {
		fail("Cannot read file", filePath)
	}

	return string(fileByteContent)
}

func processFile(fileContent string, luaState *lua.LState, writer *bufio.Writer) {
	initFileProcessing(fileContent)
	allTokens := list.New()
	for !eof() {
		tokens := getNextToken()
		tokensCount := len(tokens)
		if tokensCount == 0 {
			break
		}

		if tokens[0].tokenType == EOF {
			break
		}

		for i := 0; i < tokensCount; i++ {
			allTokens.PushBack(tokens[i])
		}
	}

	executeTokens(allTokens, luaState)
	dumpToString(allTokens, writer, luaState)
}

func initFileProcessing(fileContent string) {
	charsStack = new(CharsStack)
	content = fileContent
	contentLength = len(content)
	currentPosition = 0
	currentLineNumber = 0
}

func writeLineInformation(currentLineIndex int, currentFilePath string, writer *bufio.Writer, L *lua.LState) {
	if generateLineInfoCallback != nil {
		L.Push(generateLineInfoCallback)
		L.Push(lua.LNumber(currentLineIndex))
		L.Push(lua.LString(currentFilePath))
		L.Call(2, 1)
		returnValue := L.Get(-1).String()
		L.Pop(1)
		_, _ = writer.WriteString(returnValue + "\n")
	}
}

func dumpToString(tokens *list.List, writer *bufio.Writer, L *lua.LState) {
	actualLineIndex := 0
	currentFile = ""
	for e := tokens.Front(); e != nil; e = e.Next() {
		token := e.Value.(*Token)
		if !(token.tokenType == LUA_BLOCK_START || token.tokenType == LUA_BLOCK_END || token.tokenType == LuaBlock) {
			if actualLineIndex != token.lineIndex || currentFile != token.inputFile {
				writeLineInformation(token.lineIndex, token.inputFile, writer, L)
				actualLineIndex = token.lineIndex
				currentFile = token.inputFile
			}
			_, _ = writer.WriteString(token.value)
			linesCount := countNewLines(token.value)
			actualLineIndex += linesCount
		}
	}
	_ = writer.Flush()
}

func countNewLines(strValue string) int {
	newLinesCount := 0
	for _, _rune := range strValue {
		if _rune == '\n' {
			newLinesCount++
		}
	}
	return newLinesCount
}

func getNextToken() []*Token {
	c, success := getChar(false)
	if !success {
		return []*Token{{EOF, "", currentLineNumber, currentFile}}
	}
	if unicode.IsSpace(c) {
		return []*Token{readWhitespaceToken()}
	}
	if unicode.IsNumber(c) {
		return []*Token{readNumberToken()}
	}
	if c == '<' {
		if checkCurrentBufferContainsString(luaStartBlockMarker) {
			return readLuaBlockTokens()
		}
	}
	if c == '(' || c == ')' || c == '*' || c == '+' || c == '|' || c == '-' || c == ',' || c == '.' || c == '^' || c == '\'' || c == '"' || c == '\\' || c == '/' || c == ':' || c == ';' || c == '#' || c == '&' || c == '=' || c == '<' || c == '>' || c == '?' || c == '!' || c == '%' || c == '$' {
		skipChars(1)
		return []*Token{{SPECIAL, string(c), currentLineNumber, currentFile}}
	}
	if unicode.IsPunct(c) {
		return []*Token{readPunctToken()}
	}
	if unicode.IsLetter(c) {
		return []*Token{readSymbolToken()}
	}

	_lineNumber := currentLineNumber
	getChar(true)
	return []*Token{{UNKNOWN, string(c), _lineNumber, currentFile}}
}

func getChar(moveForward bool) (rune, bool) {
	if !charsStack.isEmpty() {
		if moveForward {
			return charsStack.pop(), true
		} else {
			return charsStack.peek(), true
		}
	}

	if eof() {
		return -1, false
	}

	result := rune(content[currentPosition])
	if moveForward {
		currentPosition++
		if result == '\n' {
			currentLineNumber++
		}
	}
	return result, true
}

func unGetChar(character rune) {
	charsStack.push(character)
	if character == '\n' {
		currentLineNumber--
	}
}

func executeTokens(tokens *list.List, luaState *lua.LState) {
	for e := tokens.Front(); e != nil; e = e.Next() {
		token := e.Value.(*Token)
		if token.tokenType == LuaBlock {
			executeLuaBlock(e, token, luaState)
		} else if token.tokenType == SYMBOL {
			macro, exists := macroMap[token.value]
			if exists {
				executeMacro(e, tokens, token, luaState, &macro)
			}
		}
	}
}

func executeMacro(tokenNode *list.Element, tokens *list.List, token *Token, luaState *lua.LState, macroStruct *MacroStruct) {
	arguments := matchArguments(tokenNode.Next(), tokens, macroStruct)
	token.value = ""
	luaState.SetGlobal("currentBlock", createUserDataFromToken(token, luaState))
	luaState.Push(macroStruct.callback)
	for _, element := range arguments {
		_, ok := element.(string)
		if ok {
			luaState.Push(lua.LString(element.(string)))
			continue
		}
		_, ok = element.([]string)
		if ok {
			var luaTable lua.LTable
			for index, _string := range element.([]string) {
				luaTable.Insert(index+1, lua.LString(_string))
			}

			luaState.Push(&luaTable)
			continue
		}
	}
	defer func() {
		if r := recover(); r != nil {
			apiError := r.(*lua.ApiError)
			token := tokenNode.Value.(*Token)
			reportErrorAndExit(token, "Error while executing lua macro [%s]\n%s", macroStruct.name, apiError.Object)
		}
	}()
	luaState.Call(len(arguments), 0)
}

func matchArguments(tokenNode *list.Element, tokens *list.List, macroStruct *MacroStruct) []interface{} {
	initToken := tokenNode
	argsCount := len(macroStruct.arguments)
	var resultList []interface{}
	if argsCount == 0 {
		//if macro has 0 arguments, it can be used as
		//macro
		//or
		//macro()
		if getTokenNodeText(tokenNode) == "(" {
			skipWhitespaces(tokenNode.Next(), tokens)
			if getTokenNodeText(tokenNode.Next()) != ")" {
				reportErrorAndExit(tokenNode.Value.(*Token), "Expected ')' to finish 0 argument list, but found [%s] while processing macro [%s]", escapeStringForDebugPrint(getTokenNodeText(tokenNode.Next())), macroStruct.name)
			}

			tokens.Remove(tokenNode.Next())
			tokens.Remove(tokenNode)
		}

		return resultList
	}

	if getTokenNodeText(tokenNode) != "(" {
		reportErrorAndExit(tokenNode.Value.(*Token), "Expected '(' to start argument list, but found [%s] while processing macro [%s]", escapeStringForDebugPrint(getTokenNodeText(tokenNode)), macroStruct.name)
	}

	tokenNode = removeNode(tokenNode, tokens)
	debugPrint(tokens, tokenNode)
	tokenNode = skipWhitespaces(tokenNode, tokens)
	debugPrint(tokens, tokenNode)
	for i := 0; i < argsCount; i++ {
		argType := macroStruct.arguments[i]
		lastArgument := i == argsCount-1
		if !macroStruct.variadic {
			if argType == "raw" {
				stringValue, _tokenNode := readNodesCollectTextUntilText(tokenNode, tokens, []string{",", ")"})
				debugPrint(tokens, tokenNode)
				tokenNode = _tokenNode
				stringValue = strings.Trim(stringValue, " \t\n\r")
				resultList = append(resultList, stringValue)
			}

			if !lastArgument {
				if getTokenNodeText(tokenNode) != "," {
					reportErrorAndExit(tokenNode.Value.(*Token), "Expected ',' but found [%s] while processing arguments of macro [%s]", escapeStringForDebugPrint(getTokenNodeText(tokenNode)), macroStruct.name)
				}
				tokenNode = removeNode(tokenNode, tokens)
				debugPrint(tokens, tokenNode)
			}
		} else {
			if !lastArgument {
				if argType == "raw" {
					stringValue, _tokenNode := readNodesCollectTextUntilText(tokenNode, tokens, []string{",", ")"})
					debugPrint(tokens, tokenNode)
					tokenNode = _tokenNode
					stringValue = strings.Trim(stringValue, " \t\n\r")
					resultList = append(resultList, stringValue)
				}

				if getTokenNodeText(tokenNode) != "," {
					reportErrorAndExit(tokenNode.Value.(*Token), "Expected ',' but found [%s]", escapeStringForDebugPrint(getTokenNodeText(tokenNode)))
				}
				tokenNode = removeNode(tokenNode, tokens)
				debugPrint(tokens, tokenNode)
			} else {
				var stringsArray []string
				for true {
					stringValue, _tokenNode := readNodesCollectTextUntilText(tokenNode, tokens, []string{",", ")"})
					debugPrint(tokens, tokenNode)
					tokenNode = _tokenNode
					stringValue = strings.Trim(stringValue, " \t\n\r")
					stringsArray = append(stringsArray, stringValue)

					nextTokenString := getTokenNodeText(tokenNode)

					if nextTokenString == ")" {
						resultList = append(resultList, stringsArray)
						break
					} else if nextTokenString == "," {
						tokenNode = removeNode(tokenNode, tokens)
						continue
					} else {
						reportErrorAndExit(tokenNode.Value.(*Token), "Cannot parse variadic argument list in macro [%s]. Expected [,] or [)] but found [%s]", macroStruct.name, escapeStringForDebugPrint(nextTokenString))
					}
				}
			}
		}
	}

	if tokenNode == nil {
		reportErrorAndExit(initToken.Value.(*Token), "Syntax error while calling macro [%s]", macroStruct.name)
		return nil
	}

	if getTokenNodeText(tokenNode) != ")" {
		reportErrorAndExit(tokenNode.Value.(*Token), "Expected ')' to finish argument list, but found [%s]", escapeStringForDebugPrint(getTokenNodeText(tokenNode)))
	}

	tokenNode = removeNode(tokenNode, tokens)
	debugPrint(tokens, tokenNode)
	return resultList
}

func readNodesCollectTextUntilText(tokenNode *list.Element, tokens *list.List, until []string) (string, *list.Element) {
	var result string
	for tokenNode != nil && !tokenEqualString(tokenNode, until) {
		result = result + getTokenNodeText(tokenNode)
		tokenNode = removeNode(tokenNode, tokens)
	}
	return result, tokenNode
}

func tokenEqualString(tokenNode *list.Element, until []string) bool {
	tokenString := getTokenNodeText(tokenNode)
	for i := 0; i < len(until); i++ {
		if tokenString == until[i] {
			return true
		}
	}
	return false
}

func skipWhitespaces(tokenNode *list.Element, tokens *list.List) *list.Element {
	for tokenNode.Value.(*Token).tokenType == WHITESPACE {
		tokenNode = removeNode(tokenNode, tokens)
		if tokenNode == nil {
			return tokenNode
		}
	}
	return tokenNode
}

func removeNode(tokenNode *list.Element, tokens *list.List) *list.Element {
	nextNode := tokenNode.Next()
	tokens.Remove(tokenNode)
	return nextNode
}

func getTokenNodeText(tokenNode *list.Element) string {
	return tokenNode.Value.(*Token).value
}

func executeLuaBlock(tokenNode *list.Element, token *Token, luaState *lua.LState) {
	luaState.SetGlobal("currentBlock", createUserDataFromToken(tokenNode.Next().Value.(*Token), luaState))
	if err := luaState.DoString(token.value); err != nil {
		apiError := err.(*lua.ApiError)
		reportErrorAndExit(token, "Error while execution lua block\n%s", apiError.Object)
	}
}

func registerFunctions(luaState *lua.LState) {
	luaState.SetGlobal("writeToBlock", luaState.NewFunction(WriteToBlock))
	luaState.SetGlobal("markBlock", luaState.NewFunction(MarkBlock))
	luaState.SetGlobal("getMarkedBlock", luaState.NewFunction(GetMarkedBlock))
	luaState.SetGlobal("macro", luaState.NewFunction(RegisterMacro))
	luaState.SetGlobal("echo", luaState.NewFunction(Echo))
	luaState.SetGlobal("registerGenerateLineInfoCallback", luaState.NewFunction(RegisterGenerateLineInfoCallback))
}

func RegisterGenerateLineInfoCallback(L *lua.LState) int {
	L.CheckFunction(1)
	generateLineInfoCallback = L.ToFunction(1)
	return 0
}

func RegisterMacro(L *lua.LState) int {
	L.CheckString(1)
	macroName := L.ToString(1)
	L.CheckTable(2)
	L.CheckFunction(3)
	_, macroExists := macroMap[macroName]
	if macroExists {
		reportErrorAndExit(nil, "Macros with name [%s] already exists", macroName)
	}

	argumentsTable := L.ToTable(2)
	var argumentsList []string
	variadicArgsFunction := false
	for i := 1; i <= argumentsTable.Len(); i++ {
		isLastArgument := i == argumentsTable.Len()

		argumentType := argumentsTable.RawGetInt(i).String()
		varargs := false
		if strings.HasSuffix(argumentType, "*") {
			varargs = true
			argumentType = argumentType[:len(argumentType)-1]
		}
		if varargs && !isLastArgument {
			reportErrorAndExit(nil, "Error while register macro [%s]. Argument %d marked as variadic, but it is not last argument", macroName, i)
		}

		if argumentType == "raw" {
			argumentsList = append(argumentsList, argumentType)
		} else {
			reportErrorAndExit(nil, "Error while register macro [%s]. Argument %d type should be [raw] but found [%s]", macroName, i, escapeStringForDebugPrint(argumentType))
		}
		if varargs {
			variadicArgsFunction = varargs
		}
	}

	callback := L.ToFunction(3)

	var macro MacroStruct
	macro.name = macroName
	macro.arguments = argumentsList
	macro.callback = callback
	macro.variadic = variadicArgsFunction
	macroMap[macroName] = macro
	return 0
}

func MarkBlock(L *lua.LState) int {
	L.CheckString(1)
	L.CheckUserData(2)

	name := L.ToString(1)
	block := L.ToUserData(2)

	_, exists := markedBlocks[name]
	if exists {
		reportErrorAndExit(nil, "Marked block with name [%s] already exists", name)
	}
	markedBlocks[name] = block.Value.(*Token)
	return 0
}

func GetMarkedBlock(L *lua.LState) int {
	L.CheckString(1)
	name := L.ToString(1)
	token, exists := markedBlocks[name]
	if !exists {
		reportErrorAndExit(nil, "Marked block with name [%s] does not exists", name)
	}
	L.Push(createUserDataFromToken(token, L))
	return 1
}

func Echo(L *lua.LState) int {
	L.CheckAny(1)
	stringValue := L.ToString(1)
	userData := L.GetGlobal("currentBlock")
	token := userData.(*lua.LUserData).Value.(*Token)
	token.value = token.value + stringValue
	return 0
}

func WriteToBlock(L *lua.LState) int {
	L.CheckUserData(1)
	L.CheckAny(2)
	blockUserData := L.ToUserData(1)
	stringValue := L.ToString(2)
	token := blockUserData.Value.(*Token)
	token.value = token.value + stringValue
	return 0
}

func createUserDataFromToken(token *Token, luaState *lua.LState) *lua.LUserData {
	userData := luaState.NewUserData()
	userData.Value = token
	return userData
}

func readLuaBlockTokens() []*Token {
	var result []*Token

	result = append(result, &Token{LUA_BLOCK_START, luaStartBlockMarker, currentLineNumber, currentFile})
	skipChars(len(luaStartBlockMarker))

	luaBlock := readTokenUntilString(LuaBlock, luaEndBlockMarker)
	if luaBlock.tokenType == EOF || luaBlock.tokenType == UNKNOWN {
		reportErrorAndExit(nil, "Read EOF while search lua block end marker ["+luaEndBlockMarker+"]. Found ["+luaBlock.value+"]")
	}

	result = append(result, luaBlock)

	result = append(result, &Token{SYMBOL, "", currentLineNumber, currentFile}) //block where script will output the text
	_lineNumber := currentLineNumber
	skipChars(len(luaEndBlockMarker))
	result = append(result, &Token{LUA_BLOCK_END, luaEndBlockMarker, _lineNumber, currentFile})
	return result
}

func escapeStringForDebugPrint(str string) string {
	str = strings.ReplaceAll(str, "\r", "\\r")
	str = strings.ReplaceAll(str, "\n", "\\n")
	str = strings.ReplaceAll(str, "\t", "\\t")

	return str
}

func reportErrorAndExit(token *Token, formatString string, args ...interface{}) {
	if token != nil {
		_, _ = fmt.Fprintf(os.Stderr, "Error at %s:%d\n", token.inputFile, token.lineIndex+1)
	}
	_, _ = fmt.Fprintf(os.Stderr, formatString, args...)
	_, _ = fmt.Fprintln(os.Stderr)
	os.Exit(1)
}
