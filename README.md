#godiff
A File/Directory diff-like comparision tool with HTML output.

This program can be use to compare files and directories for differences.
When comparing directories, it iterates through all files in both directories
and compare files having the same name.

See [example](example.html) output.

##How to Use
	godiff file1 file2 > results.html
	godiff directory1 directory > results.html

See `godiff -h` for all the available command line options

##Features
* When comparing two directory, place all the differences into  a single html file.
* Supports UTF8 file. 
* Show differences within a line
* Options for ignore case, white spaces compare, blank lines etc.

##Go Language
This program is created in the go language.
Download go from [golang.org](http://golang.org)

##How to Build
On Linux or Darwin OS
	go build -o godiff godiff.go godiff_unix.go

On Windows
	go build -o godiff.exe  godiff.go godiff_windows.go

