godiff
======

File/Directory diff tool with HTML output
Copyright (C) 2012   Siu Pin Chao

 This program is free software: you can redistribute it and/or modify
 it under the terms of the GNU General Public License as published by
 the Free Software Foundation, either version 3 of the License, or
 (at your option) any later version.

 This program is distributed in the hope that it will be useful,
 but WITHOUT ANY WARRANTY; without even the implied warranty of
 MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE.  See the
 GNU General Public License for more details.

 You should have received a copy of the GNU General Public License
 along with this program.  If not, see <http://www.gnu.org/licenses/>.

Description:
 This program can be use to compare files and directories for differences.
 When comparing directories, it iterates through all files in both directories
 and compare files having the same name.
 
 It uses the algorithm from "An O(ND) Difference Algorithm and its Variations" 
 by Eugene Myers Algorithmica Vol. 1 No. 2, 1986, p 251. 

Main Features:
 * Supports UTF8 file. 
 * Show differences within a line
 * Options for ignore case, white spaces compare, blank lines etc.

Main aim of the application is to try out the features in the go programming language. (golang.org)
 * Slices: Used extensively, and re-slicing too whenever it make sense.
 * File I/O: Use Mmap for reading text files
 * Function Closure: Use in callbacks functions to handle both file and line compare
 * Goroutines: for running multiple file compares concurrently, using channels and mutex too.

How to Compile:
 Download and install go from golang.go. 
 Run the command "go build godiff.go" to build it

