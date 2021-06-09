**LuaTextProcessor** is application that can work like simple preprocessor for any txt file.
You can register lua functions that can be called from txt files and they can produce some output.

# Quick start

Usage:
 **luatp -f FILE_PATH_1 -f FILE_PATH_N -o OUTPUT_FILE_PATH**

**-f** - input file path. You can provide any number of input files. They will be processed in order. Any input file can contain lua declaration blocks

**-l** - lua file path. Lua files can store some utility functions to make your input files cleaner. You can provide any number of lua files. They will be processed in order.

**-o** - output file path. Also it accepts *console* to write the result to stdout. *console* is a default value in case if this flag is omitted.  

Basic example:

*myfile.txt*
```lua
<?lua
    --Example of macro without arguments
    macro('printdate',{}, function()
        echo(os.date("today is %A, in %B"))
    end)

    --Example of macro with one argument
    local int2str ={
        "one", "two","three","four","five","six","seven","eight","nine"
    }
    macro('INT_TO_STR',{"raw"}, function(num)
        echo(int2str[tonumber(num)])
    end)

    --example of macro with two arguments
    macro('SUM',{"raw","raw"}, function(x1,x2)
        echo("sum="..(tonumber(x1)+tonumber(x2)))
    end)

    --example of macro with variable arguments
    macro('PRINT_LIST',{"raw","raw*"}, function(listName,arrayOfRestOfArgs)
        echo("Printing list "..listName.."\n")
        for i=1,#arrayOfRestOfArgs do
            echo(" * "..arrayOfRestOfArgs[i].."\n")
        end
    end)
lua?>some text
no arguments: printdate()
one argument: INT_TO_STR(8)
two arguments: SUM(8,3)
variable number of arguments: PRINT_LIST(my list, element 1, "el 2", 4322)
sometext
```
Process the file with the following command: 

```luatp -f myfile.txt```

Received output:
```
some text
no arguments: today is Wednesday, in June
one argument: eight
two arguments: sum=11
variable number of arguments: Printing list my list
 * element 1
 * "el 2"
 * 4322

sometext
```

The *myfile.txt* of course can be split on any number of files, for example *lua_definitions.txt* and *content.txt* and executed with the following command:
```luatp -f lua_definitions.txt -f content.txt``` 
the output should be the same.

# Functions

**macro(name, argsInfo, callback(callback_args))** - declare macro that you can call from text blocks.

* **name** -name of macro that is used to call it
* **argsInfo** - table that declares number of arguments and their types. Currently only *raw* type is supported. Variable arguments number is supported. Examples:
  * No args ```{}```
  * One arg ```{"raw"}```
  * Three args ```{"raw","raw","raw"}``` 
  * Variable number of args ```{"raw*"}``` - in this case all provided args will be joined to table and passed to your callback as single argument
  * Fixed args and variable number of args ```{"raw","raw","raw*"}``` - in this case *arg0* and *arg1* will be passed to own callback's arguments, but *arg2*-*argN* will be joined to table and passed to third callback argument
* **callback** - lua callback that will be executed when processor will encounter it's invocation in text. Should have the same number of arguments as argsInfo. Received values in args always have *string* type, except variable arg that will be array of strings.
Callback function should not return any value

**echo(string)** - write text to the output

**markBlock(str_key, block_reference)** - mark text block with some string key to be able to reference it later. There is global variable **currentBlock** that always references to current text block

**getMarkedBlock(str_key)** - get reference to block that was marked with **markBlock**     

**writeToBlock(block_reference, string)** - append text to text block. Often used with getMarkedBlock()

**registerGenerateLineInfoCallback(callback(lineIndex, filePath))** - this function allows to register callback that can return some string that will be used to generate #line directives for compilers or assemblers in case if line numbering goes out of sync(for example when lua text blocks have been removed). Example:
  ```lua
<?lua
	registerGenerateLineInfoCallback(function(lineIndex, filePath)
	    return "#line lineIndex:"..lineIndex.." filePath:"..filePath
	end)
lua?>1
<?lua
	


lua?>2
```
Output:
```
#line lineIndex:4 filePath:./example/test.txt
1
#line lineIndex:9 filePath:./example/test.txt
2
```

## Mark and write functions example

For example we want to preprocess assembler file and we do not want to write all strings in data section, but just write them inplace.
To achieve this goal we can define following lua macros:

```lua
macro("STRINGS_LITERALS_STORAGE",{},function()
    markBlock("strings_literals_storage",currentBlock)
end)

macro("STRING_LITERAL",{"raw","raw"},function(varName, str)
    writeToBlock(getMarkedBlock("strings_literals_storage"), ""..varName.."  db  "..str..", 0\n")
end)
```

And then in your program you can use it like this:
```asm
.data
STRINGS_LITERALS_STORAGE
.code
STRING_LITERAL(helloWorld, "Hello world!")
STRING_LITERAL(mystring, "Another string!")
lea si, helloWorld
```

This input file will be preprocessed into following assembler source file
```asm
.data
helloWorld db "Hello world!",0
mystring db "Another string!",0
.code
lea si, helloWorld
```

This example shows important feature of the application - it does not process files in stream way, it buffers everything in memory and only at the end dump everything to output. That is why it is possible to append text to already processed blocks