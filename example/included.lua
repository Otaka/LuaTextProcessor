macro('mymacro',{"raw","raw*"}, function(a,arrExampl)
    echo("Executed my macro "..a.."\n")
    for i=1, #arrExampl do
        echo("   arg "..arrExampl[i].."\n")
    end
end)
registerGenerateLineInfoCallback(function(lineIndex, filePath)
    return "#line lineIndex:"..lineIndex.." filePath:"..filePath
end)