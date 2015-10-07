#godiff
A File/Directory diff-like comparison tool with HTML output.

This program can be use to compare files and directories for differences.
When comparing directories, it iterates through all files in both directories
and compare files having the same name.

See example output [here:](http://raw.githack.com/spcau/godiff/master/example.html)

##How to use godiff

	godiff file1 file2 > results.html
	godiff directory1 directory > results.html

See `godiff -h` for all the available command line options

##Features

* When comparing two directory, place all the differences into a single html file.
* Supports UTF8 file.
* Show differences within a line
* Options for ignore case, white spaces compare, blank lines etc.

##Description

I need a program to to compare 2 directories, and report differences in all
files. Much like gnudiff, but with a nicer output. And I also like to try out
the go programming language, so I created __godiff__.

The _diff_ algorithm implemented here is based on 
_"An O(ND) Difference Algorithm and its Variations"_
by Eugene Myers Algorithmica Vol. 1 No. 2, 1986, p 251. 

__godiff__ always tries to produce the minimal differences, 
just like gnudiff with the "-d" option.

##Go Language

This program is created in the go programming language.
For more information about _go_, see [golang.org](http://golang.org)

##How to Build

On Linux or Darwin OS

	go build -o godiff godiff.go godiff_unix.go

On Windows

	go build -o godiff.exe  godiff.go godiff_windows.go

