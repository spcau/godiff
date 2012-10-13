//
//  File/Directory diff tool with HTML output
//  Copyright (C) 2012   Siu Pin Chao
//
//  This program is free software: you can redistribute it and/or modify
//  it under the terms of the GNU General Public License as published by
//  the Free Software Foundation, either version 3 of the License, or
//  (at your option) any later version.
//
//  This program is distributed in the hope that it will be useful,
//  but WITHOUT ANY WARRANTY; without even the implied warranty of
//  MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE.  See the
//  GNU General Public License for more details.
//
//  You should have received a copy of the GNU General Public License
//  along with this program.  If not, see <http://www.gnu.org/licenses/>.
//
// Description:
//  This program can be use to compare files and directories for differences.
//  When comparing directories, it iterates through all files in both directories
//  and compare files having the same name.
//  
//  It uses the algorithm from "An O(ND) Difference Algorithm and its Variations" 
//  by Eugene Myers Algorithmica Vol. 1 No. 2, 1986, p 251. 
// 
// Main Features:
//  * Supports UTF8 file. 
//  * Show differences within a line
//  * Options for ignore case, white spaces compare, blank lines etc.
//
// Main aim of the application is to try out the features in the go programming language. (golang.org)
//  * Slices: Used extensively, and re-slicing too whenever it make sense.
//  * File I/O: Use Mmap for reading text files
//  * Function Closure: Use in callbacks functions to handle both file and line compare
//  * Goroutines: for running multiple file compares concurrently, using channels and mutex too.
//
// How to Compile:
//  Download and install go from golang.go. 
//  Run the command "go build godiff.go" to build it
//
//  History
//  -------
//  2012/09/20  Created
//
//
package main

import (
	"bufio"
	"bytes"
	"flag"
	"fmt"
	"html"
	"os"
	"runtime"
	"runtime/pprof"
	"sort"
	"strings"
	"sync"
	"syscall"
	"time"
	"unicode"
	"unicode/utf8"
)

const VERSION = "0.01"

// Scan at up to this size in file for '\0' in test for binary file
const BINARY_CHECK_SIZE = 65536

// Output buffer size
const OUTPUT_BUF_SIZE = 100000

// default number of context lines to display
const CONTEXT_LINES = 3

// convenient shortcut
const PATH_SEPARATOR = string(os.PathSeparator)

// use mmap for file greather than this size
const MMAP_THRESHOLD = 200000

// Mmap'ed file 
type Filedata struct {
	name       string
	info       os.FileInfo
	handle     *os.File
	errormsg   string
	is_binary  bool
	is_mmapped bool
	data       []byte
}

// Callback funcs for diff.
// The actual callbaks are implemented with function closures to enable it ot access private data
type DiffAction struct {
	diff_same   func(int, int, int, int)
	diff_modify func(int, int, int, int)
	diff_insert func(int, int, int, int)
	diff_remove func(int, int, int, int)
}

// Output to diff as html or text format
type OutputFormat struct {
	line1_start, line1_end int
	line2_start, line2_end int
	buf1, buf2             bytes.Buffer
	name1, name2           string
	fileinfo1, fileinfo2   os.FileInfo
	header_printed         bool
}

// Messages
const MSG_FILE_SIZE_ZERO = "File has size 0"
const MSG_FILE_NOT_EXISTS = "File does not exist"
const MSG_DIR_NOT_EXISTS = "Directory does not exist"
const MSG_FILE_IS_BINARY = "This is a binary file"
const MSG_BIN_FILE_DIFFERS = "Binary file differs"
const MSG_FILE_IDENTICAL = "Files are the same"
const MSG_FILE_TOO_BIG = "File too big"

const HTML_HEADER = `<!doctype html><html><head>
<meta http-equiv="content-type" content="text/html;charset=utf-8">`

const HTML_CSS = `<style type="text/css">
.tab {border-color:#808080; border-style:solid; border-width:1px 1px 1px 1px; border-collapse:collapse;}
.tth {border-color:#808080; border-style:solid; border-width:1px 1px 1px 1px; border-collapse:collapse; padding:4px; vertical-align:top; text-align:left; background-color:#E0E0E0;}
.ttd {border-color:#808080; border-style:solid; border-width:1px 1px 1px 1px; border-collapse:collapse; padding:4px; vertical-align:top; text-align:left;}
.hdr {color:black; font-size:85%;}
.inf {color:#C08000; font-size:85%;}
.err {color:red; font-size:85%; font-style:bold; margin:0; display:block;}
.msg {color:#508050; font-size:85%; font-style:bold; margin:0; display:block;}
.lin {color:#C08000; font-size:75%; font-style:italic; margin:0; display:block;}
.nop {color:black; font-size:75%; font-family:monospace; white-space:pre; margin:0; display:block;}
.upd {color:black; font-size:75%; font-family:monospace; white-space:pre; margin:0; background-color:#CFCFFF; display:block;}
.chg {color:#C00080;}
.add {color:black; font-size:75%; font-family:monospace; white-space:pre; margin:0; background-color:#CFFFCF; display:block;}
.del {color:black; font-size:75%; font-family:monospace; white-space:pre; margin:0; background-color:#FFCFCF; display:block;}
</style>`

const HTML_LEGEND = `<br><b>Legend:</b><br><table class="tab">
<tr><td class="tth"><span class="hdr">filename 1</span></td><td class="tth"><span class="hdr">filename 2</span></td></tr>
<tr><td class="ttd">
<span class="lin">Line N</span>
<span class="del">  line deleted</span>
<span class="nop">  no change</span>
<span class="upd">  line modified</span>
</td>
<td class="ttd">
<span class="lin">Line M</span>
<span class="add">  line added</span>
<span class="nop">  no change</span>
<span class="upd">  <span class="chg">L</span>ine <span class="chg">M</span>odified</span>
</td></tr>
</table>
`

// command line arguments
var flag_pprof_file string
var flag_version bool = false
var flag_cmp_ignore_case bool = false
var flag_cmp_ignore_blank_lines bool = false
var flag_cmp_ignore_space_change bool = false
var flag_cmp_ignore_all_space bool = false
var flag_unicode_case_and_space bool = false
var flag_show_identical_files bool = false
var flag_suppress_line_changes bool = false
var flag_suppress_missing_file bool = false
var flag_output_as_text bool = false
var flag_context_lines int = CONTEXT_LINES
var flag_max_goroutines = 1

// Job queue for goroutines
type JobQueue struct {
	name1, name2 string
	info1, info2 os.FileInfo
}

var job_queue chan JobQueue
var job_wait sync.WaitGroup

// buffered stdout.
var out = bufio.NewWriterSize(os.Stdout, OUTPUT_BUF_SIZE)
var out_lock sync.Mutex

// html entity strings
var html_entity_amp = html.EscapeString("&")
var html_entity_gt = html.EscapeString(">")
var html_entity_lt = html.EscapeString("<")
var html_entity_single_quote = html.EscapeString("'")
var html_entity_double_quote = html.EscapeString("\"")

func version() {
	fmt.Printf("godiff. Version %s\n", VERSION)
	fmt.Printf("Copyright (C) 2012 Siu Pin Chao.\n")
}

func usage(msg string) {
	if msg != "" {
		fmt.Fprintf(os.Stderr, "%s\n", msg)
	}
	fmt.Fprint(os.Stderr, "A text file comparison tool displaying differenes in HTML\n\n")
	fmt.Fprint(os.Stderr, "usage: godiff <options> <file|dir> <file|dir>\n")
	flag.PrintDefaults()
	out.Flush()
	os.Exit(2)
}

func usage0() {
	usage("")
}

// functions to compare line and computer hash values,
// these will be setup based on flags: -b -w -U etc.
var compare_line func([]byte, []byte) bool
var compute_hash func([]byte) uint32

// Main routine.
func main() {

	// setup command line options
	flag.Usage = usage0
	flag.StringVar(&flag_pprof_file, "prof", "", "Write pprof output to file")
	flag.BoolVar(&flag_version, "v", flag_version, "Print version information")
	flag.IntVar(&flag_context_lines, "c", flag_context_lines, "Include N lines of context before and after changes")
	flag.IntVar(&flag_max_goroutines, "g", flag_max_goroutines, "Max number of goroutines to use for file comparison")
	flag.BoolVar(&flag_cmp_ignore_space_change, "b", flag_cmp_ignore_space_change, "Ignore changes in the amount of white space")
	flag.BoolVar(&flag_cmp_ignore_all_space, "w", flag_cmp_ignore_all_space, "Ignore all white space")
	flag.BoolVar(&flag_cmp_ignore_case, "i", flag_cmp_ignore_case, "Ignore case differences in file contents")
	flag.BoolVar(&flag_cmp_ignore_blank_lines, "B", flag_cmp_ignore_blank_lines, "Ignore changes whose lines are all blank")
	flag.BoolVar(&flag_unicode_case_and_space, "unicode", flag_unicode_case_and_space, "Apply unicode rules for white space and upper/lower case")
	flag.BoolVar(&flag_show_identical_files, "s", flag_show_identical_files, "Report when two files are the identical")
	flag.BoolVar(&flag_suppress_line_changes, "l", flag_suppress_line_changes, "Do not display changes within lines")
	flag.BoolVar(&flag_suppress_missing_file, "m", flag_suppress_missing_file, "Do not show content if corresponding file is missing")
	flag.BoolVar(&flag_output_as_text, "n", flag_output_as_text, "Output using 'diff' text format instead of HTML")
	flag.Parse()

	if flag_version {
		version()
		os.Exit(0)
	}

	if flag_pprof_file != "" {
		pf, err := os.Create(flag_pprof_file)
		if err != nil {
			usage(err.Error())
		}
		pprof.StartCPUProfile(pf)
		defer pprof.StopCPUProfile()
	}

	// flush output on termination
	defer func() {
		out.Flush()
		out = nil
	}()

	// choose which compare hash hash function to use
	if flag_cmp_ignore_case || flag_cmp_ignore_space_change || flag_cmp_ignore_all_space {
		if flag_unicode_case_and_space {
			compute_hash = compute_hash_unicode
			compare_line = compare_line_unicode
		} else {
			compute_hash = compute_hash_bytes
			compare_line = compare_line_bytes
		}
	} else {
		compute_hash = compute_hash_exact
		compare_line = bytes.Equal
	}

	// get command line args
	args := flag.Args()
	if len(args) < 2 {
		usage("Missing files")
	}

	// check file type
	file1, file2 := args[0], args[1]
	finfo1, err1 := os.Stat(file1)
	finfo2, err2 := os.Stat(file2)

	// Unable to find either file/directory
	if err1 != nil || err2 != nil {
		if err1 != nil {
			fmt.Fprintf(os.Stderr, "%s\n", err1.Error())
		}
		if err2 != nil {
			fmt.Fprintf(os.Stderr, "%s\n", err2.Error())
		}
		os.Exit(1)
	}

	if finfo1.IsDir() != finfo2.IsDir() {
		usage("Unable to compare file and directory")
	}

	if !flag_output_as_text {
		out.WriteString(HTML_HEADER)
		fmt.Fprintf(out, "<title>Compare %s vs %s</title>\n", html.EscapeString(file1), html.EscapeString(file2))
		out.WriteString(HTML_CSS)
		out.WriteString("</head>")
		out.WriteString("<body>\n")
		fmt.Fprintf(out, "<h1>Compare %s vs %s</h1><br>\n", html.EscapeString(file1), html.EscapeString(file2))
	}

	switch {
	case !finfo1.IsDir() && !finfo2.IsDir():
		diff_file(file1, file2, finfo1, finfo2)

	case finfo1.IsDir() && finfo2.IsDir():
		job_queue_init()
		diff_dirs(file1, file2, finfo1, finfo2)
		job_queue_finish()
	}

	if !flag_output_as_text {
		fmt.Fprintf(out, "Generated on %s<br>", time.Now().Format(time.RFC1123))
		out.WriteString(HTML_LEGEND)
		out.WriteString("</body>")
		out.WriteString("</html>\n")
	}
}

//
// Call the diff algorithm.
//
func do_diff(data1, data2 []int) ([]bool, []bool) {
	len1, len2 := len(data1), len(data2)
	change1, change2 := make([]bool, len1), make([]bool, len2)

	size := (len1+len2+1)*2 + 2
	v := make([]int, size*2)

	// Run diff compare algorithm.
	changed := algorithm_lcs(data1, data2, change1, change2, v)

	// No change, return nil
	if !changed {
		change1, change2 = nil, nil
	}
	return change1, change2
}

//
// Report diff changes.
// For each type of change, call the corresponding 'action' function
//
func report_changes(action *DiffAction, data1, data2 []int, change1, change2 []bool) {
	len1, len2 := len(change1), len(change2)
	i1, i2 := 0, 0

	// scan for changes
	for i1 < len1 || i2 < len2 {
		switch {
		case i1 < len1 && i2 < len2 && !change1[i1] && !change2[i2]:
			s1, s2 := i1+1, i2+1
			for s1 < len1 && s2 < len2 && !change1[s1] && !change2[s2] {
				s1, s2 = s1+1, s2+1
			}
			if action.diff_same != nil {
				action.diff_same(i1, s1, i2, s2)
			}
			i1, i2 = s1, s2

		case i1 < len1 && i2 < len2 && change1[i1] && change2[i2] && action.diff_modify != nil:
			s1, s2 := i1+1, i2+1
			for s1 < len1 && change1[s1] {
				s1++
			}
			for s2 < len2 && change2[s2] {
				s2++
			}
			for i1 < s1 && data1[i1] == 0 {
				i1++
			}
			for i2 < s2 && data2[i2] == 0 {
				i2++
			}
			if i1 < s1 && i2 < s2 {
				action.diff_modify(i1, s1, i2, s2)
			} else if i1 < s1 {
				action.diff_remove(i1, s1, i2, i2)
			} else if i2 < s2 {
				action.diff_insert(i1, i1, i2, s2)
			}
			i1, i2 = s1, s2

		case i1 < len1 && change1[i1]:
			s1 := i1 + 1
			for s1 < len1 && change1[s1] {
				s1++
			}
			for i1 < s1 && data1[i1] == 0 {
				i1++
			}
			if i1 < s1 {
				action.diff_remove(i1, s1, i2, i2)
			}
			i1 = s1

		case i2 < len2 && change2[i2]:
			s2 := i2 + 1
			for s2 < len2 && change2[s2] {
				s2++
			}
			for i2 < s2 && data2[i2] == 0 {
				i2++
			}
			if i2 < s2 {
				action.diff_insert(i1, i1, i2, s2)
			}
			i2 = s2

		default: // should not reach here
			os.Exit(4)
		}
	}
}

func to_lower_byte(b byte) byte {
	if b >= 'A' && b <= 'Z' {
		return b - 'A' + 'a'
	}
	return b
}

// split text into array of individual rune, and another array for comparison.
func split_runes(s []byte) ([]rune, []int) {

	data := make([]rune, len(s))
	cmp := make([]int, len(s))

	i, n := 0, 0
	var r rune
	var h int

	for i < len(s) {
		if s[i] < utf8.RuneSelf {
			r = rune(s[i])
			if flag_cmp_ignore_case {
				if flag_unicode_case_and_space {
					h = int(unicode.ToLower(r))
				} else {
					h = int(to_lower_byte(byte(r)))
				}
			} else {
				h = int(r)
			}
			i++
		} else {
			r, rsize := utf8.DecodeRune(s[i:])
			if flag_cmp_ignore_case && flag_unicode_case_and_space {
				h = int(unicode.ToLower(r))
			} else {
				h = int(r)
			}
			i += rsize
		}

		data[n], cmp[n] = r, h
		n = n + 1
	}
	return data[:n], cmp[:n]
}

func write_html_rune(buf *bytes.Buffer, r rune) {

	if r < utf8.RuneSelf {
		switch r {
		case '<':
			buf.WriteString(html_entity_lt)
		case '>':
			buf.WriteString(html_entity_gt)
		case '&':
			buf.WriteString(html_entity_amp)
		case '\'':
			buf.WriteString(html_entity_single_quote)
		case '"':
			buf.WriteString(html_entity_double_quote)
		default:
			buf.WriteByte(byte(r))
		}
	} else {
		buf.WriteRune(r)
	}
}

//
// Write bytes to buffer, ready to be output as html,
// replace special chars with html-entities
//
func write_html_bytes(buf *bytes.Buffer, line []byte) {

	var v rune
	var size int
	var esc string = ""

	i := 0
	lasti := 0

	for i < len(line) {
		v = rune(line[i])
		if v < utf8.RuneSelf {
			size = 1
			switch v {
			case '<':
				esc = html_entity_lt
			case '>':
				esc = html_entity_gt
			case '&':
				esc = html_entity_amp
			case '\'':
				esc = html_entity_single_quote
			case '"':
				esc = html_entity_double_quote
			}
		} else {
			v, size = utf8.DecodeRune(line[i:])
		}

		if esc != "" {
			if lasti < i {
				buf.Write(line[lasti:i])
			}
			lasti = i + size
			buf.WriteString(esc)
			esc = ""
		}

		i += size
	}

	buf.Write(line[lasti:])
}

func output_diff_message(filename1, filename2 string, info1, info2 os.FileInfo, msg1, msg2 string, is_error bool) {
	output_diff_message_content(filename1, filename2, info1, info2, msg1, msg2, nil, nil, is_error)
}

func output_diff_message_content(filename1, filename2 string, info1, info2 os.FileInfo, msg1, msg2 string, data1, data2 []byte, is_error bool) {

	if flag_output_as_text {
		out_acquire_lock()
		fmt.Fprintf(out, "#< %s: %s\n", filename1, msg1)
		fmt.Fprintf(out, "#> %s: %s\n\n", filename2, msg2)
		out_release_lock()
	} else {

		outfmt := OutputFormat{
			name1:     filename1,
			name2:     filename2,
			fileinfo1: info1,
			fileinfo2: info2,
		}

		var id string
		if is_error {
			id = "err"
		} else {
			id = "msg"
		}

		if msg1 != "" {
			outfmt.buf1.WriteString("<span class=\"")
			outfmt.buf1.WriteString(id)
			outfmt.buf1.WriteString("\">")
			write_html_bytes(&outfmt.buf1, []byte(msg1))
			outfmt.buf1.WriteString("</span>")
		} else if data1 != nil && len(data1) > 0 {

			outfmt.buf1.WriteString("<span class=\"nop\">")
			write_html_bytes(&outfmt.buf1, data1)
			outfmt.buf1.WriteString("</span>")
		}

		if msg2 != "" {
			outfmt.buf2.WriteString("<span class=\"")
			outfmt.buf2.WriteString(id)
			outfmt.buf2.WriteString("\">")
			write_html_bytes(&outfmt.buf2, []byte(msg2))
			outfmt.buf2.WriteString("</span>")
		} else if data2 != nil && len(data2) > 0 {

			outfmt.buf2.WriteString("<span class=\"nop\">")
			write_html_bytes(&outfmt.buf2, data2)
			outfmt.buf2.WriteString("</span>")
		}

		html_add_block(&outfmt)
		if outfmt.header_printed {
			out.WriteString("</table><br>\n")
			outfmt.header_printed = false
			out_release_lock()
		}
	}
}

func html_add_block(outfmt *OutputFormat) {

	if outfmt.buf1.Len() > 0 || outfmt.buf2.Len() > 0 || outfmt.line1_start < outfmt.line1_end || outfmt.line2_start < outfmt.line2_end {

		if !outfmt.header_printed {
			out_acquire_lock()
			outfmt.header_printed = true
			out.WriteString("<table class=\"tab\">\n")
			out.WriteString("<tr><td class=\"tth\"><span class=\"hdr\">")
			out.WriteString(html.EscapeString(outfmt.name1))
			out.WriteString("</span>")
			if outfmt.fileinfo1 != nil {
				fmt.Fprintf(out, "<br><span class=\"inf\">%d", outfmt.fileinfo1.Size())
				fmt.Fprintf(out, " %s</span>", outfmt.fileinfo1.ModTime().Format(time.RFC1123))
			}

			out.WriteString("</td><td class=\"tth\"><span class=\"hdr\">")
			out.WriteString(html.EscapeString(outfmt.name2))
			out.WriteString("</span>")
			if outfmt.fileinfo2 != nil {
				fmt.Fprintf(out, "<br><span class=\"inf\">%d", outfmt.fileinfo2.Size())
				fmt.Fprintf(out, " %s</span>", outfmt.fileinfo2.ModTime().Format(time.RFC1123))
			}
			out.WriteString("</td></tr>")
		}

		out.WriteString("<tr><td class=\"ttd\">")
		if outfmt.line1_start < outfmt.line1_end {
			fmt.Fprintf(out, "<span class=\"lin\">Line %d to %d</span>", outfmt.line1_start+1, outfmt.line1_end)
		}
		out.Write(outfmt.buf1.Bytes())
		out.WriteString("</td><td class=\"ttd\">")
		if outfmt.line2_start < outfmt.line2_end {
			fmt.Fprintf(out, "<span class=\"lin\">Line %d to %d</span>", outfmt.line2_start+1, outfmt.line2_end)
		}
		out.Write(outfmt.buf2.Bytes())
		out.WriteString("</td></tr>\n")
	}

	outfmt.buf1.Reset()
	outfmt.buf2.Reset()

	outfmt.line1_start = -1
	outfmt.line2_start = -1
}

func html_add_context_lines(outfmt *OutputFormat, data1, data2 [][]byte, line1, line2 int) {

	var end1, end2 int

	// Add 'context' lines after the last 'diff' and before this 'diff' segment
	if outfmt.line1_end > 0 || outfmt.line2_end > 0 {
		end1 = outfmt.line1_end + flag_context_lines
		end2 = outfmt.line2_end + flag_context_lines

		if end1 > len(data1) {
			end1 = len(data1)
		}
		if end2 > len(data2) {
			end2 = len(data2)
		}

		if end1 < line1 && end2 < line2 {
			outfmt.buf1.WriteString("<span class=\"nop\">")
			for end1 > outfmt.line1_end {
				write_html_bytes(&outfmt.buf1, data1[outfmt.line1_end])
				outfmt.buf1.WriteByte('\n')
				outfmt.line1_end++
			}
			outfmt.buf1.WriteString("</span>")

			outfmt.buf2.WriteString("<span class=\"nop\">")
			for end2 > outfmt.line2_end {
				write_html_bytes(&outfmt.buf2, data2[outfmt.line2_end])
				outfmt.buf2.WriteByte('\n')
				outfmt.line2_end++
			}
			outfmt.buf2.WriteString("</span>")
		}
	}

	if line1 >= len(data1) && line2 >= len(data2) {
		return
	}

	end1 = line1 - flag_context_lines
	end2 = line2 - flag_context_lines

	if end1 > 0 && end2 > 0 && end1 > outfmt.line1_end && end2 > outfmt.line2_end {

		html_add_block(outfmt)

		outfmt.line1_end = end1
		outfmt.line2_end = end2
	}

	if outfmt.line1_start < 0 {
		outfmt.line1_start = outfmt.line1_end
	}
	if outfmt.line2_start < 0 {
		outfmt.line2_start = outfmt.line2_end
	}

	if line1 > outfmt.line1_end {
		outfmt.buf1.WriteString("<span class=\"nop\">")
		for line1 > outfmt.line1_end {
			write_html_bytes(&outfmt.buf1, data1[outfmt.line1_end])
			outfmt.buf1.WriteByte('\n')
			outfmt.line1_end++
		}
		outfmt.buf1.WriteString("</span>")
	}

	if line2 > outfmt.line2_end {
		outfmt.buf2.WriteString("<span class=\"nop\">")
		for line2 > outfmt.line2_end {
			write_html_bytes(&outfmt.buf2, data2[outfmt.line2_end])
			outfmt.buf2.WriteByte('\n')
			outfmt.line2_end++
		}
		outfmt.buf2.WriteString("</span>")
	}
}

func diff_html_insert(outfmt *OutputFormat, data1, data2 [][]byte, start1, end1, start2, end2 int) {

	html_add_context_lines(outfmt, data1, data2, start1, start2)
	outfmt.buf2.WriteString("<span class=\"add\">")
	for start2 < end2 {
		write_html_bytes(&outfmt.buf2, data2[start2])
		outfmt.buf2.WriteByte('\n')
		start2++
	}
	outfmt.buf2.WriteString("</span>")
	outfmt.line2_end = end2
}

func diff_html_remove(outfmt *OutputFormat, data1, data2 [][]byte, start1, end1, start2, end2 int) {

	html_add_context_lines(outfmt, data1, data2, start1, start2)
	outfmt.buf1.WriteString("<span class=\"del\">")
	for start1 < end1 {
		write_html_bytes(&outfmt.buf1, data1[start1])
		outfmt.buf1.WriteByte('\n')
		start1++
	}
	outfmt.buf1.WriteString("</span>")
	outfmt.line1_end = end1
}

func diff_html_modify(outfmt *OutputFormat, data1, data2 [][]byte, start1, end1, start2, end2 int) {

	html_add_context_lines(outfmt, data1, data2, start1, start2)

	outfmt.buf1.WriteString("<span class=\"upd\">")
	outfmt.buf2.WriteString("<span class=\"upd\">")

	for start1 < end1 && start2 < end2 {

		if flag_suppress_line_changes {

			write_html_bytes(&outfmt.buf1, data1[start1])
			write_html_bytes(&outfmt.buf2, data2[start2])

		} else {

			rline1, rcmp1 := split_runes(data1[start1])
			rline2, rcmp2 := split_runes(data2[start2])

			change1, change2 := do_diff(rcmp1, rcmp2)

			if change1 != nil {

				// perform shift boundaries, to make the changes more readable
				shift_boundaries(rcmp1, change1, rune_bouundary_score)
				shift_boundaries(rcmp2, change2, rune_bouundary_score)

				action := DiffAction{}

				action.diff_insert = func(start1, end1, start2, end2 int) {
					outfmt.buf2.WriteString("<span class=\"chg\">")
					for start2 < end2 {
						write_html_rune(&outfmt.buf2, rline2[start2])
						start2++
					}
					outfmt.buf2.WriteString("</span>")
				}

				action.diff_remove = func(start1, end1, sart2, end2 int) {
					outfmt.buf1.WriteString("<span class=\"chg\">")
					for start1 < end1 {
						write_html_rune(&outfmt.buf1, rline1[start1])
						start1++
					}
					outfmt.buf1.WriteString("</span>")
				}

				action.diff_same = func(start1, end1, start2, end2 int) {
					for start1 < end1 {
						write_html_rune(&outfmt.buf1, rline1[start1])
						start1++
					}
					for start2 < end2 {
						write_html_rune(&outfmt.buf2, rline2[start2])
						start2++
					}
				}

				report_changes(&action, rcmp1, rcmp2, change1, change2)
			}
		}

		outfmt.buf1.WriteByte('\n')
		outfmt.buf2.WriteByte('\n')

		start1++
		start2++
	}

	outfmt.buf1.WriteString("</span>")
	outfmt.buf2.WriteString("</span>")
	outfmt.line1_end = start1
	outfmt.line2_end = start2

	if start1 < end1 {
		outfmt.buf1.WriteString("<span class=\"del\">")
		for start1 < end1 {
			write_html_bytes(&outfmt.buf1, data1[start1])
			outfmt.buf1.WriteByte('\n')
			start1++
		}
		outfmt.buf1.WriteString("</span>")
		outfmt.line1_end = end1
	}

	if start2 < end2 {
		outfmt.buf2.WriteString("<span class=\"add\">")
		for start2 < end2 {
			write_html_bytes(&outfmt.buf2, data2[start2])
			outfmt.buf2.WriteByte('\n')
			start2++
		}
		outfmt.buf2.WriteString("</span>")
		outfmt.line2_end = end2
	}
}

func diff_text_header(outfmt *OutputFormat) {
	if !outfmt.header_printed {
		out_acquire_lock()
		outfmt.header_printed = true
		fmt.Fprintf(out, "#< %s\n", outfmt.name1)
		fmt.Fprintf(out, "#> %s\n", outfmt.name2)
	}
}

func diff_text_modify(outfmt *OutputFormat, data1, data2 [][]byte, start1, end1, start2, end2 int) {
	diff_text_header(outfmt)
	switch {
	case end1-start1 == 1 && end2-start2 == 1:
		fmt.Fprintf(out, "%dc%d\n", start1+1, start2+1)
	case end1-start1 == 1:
		fmt.Fprintf(out, "%dc%d,%d\n", start1+1, start2+1, end2)
	case end2-start2 == 1:
		fmt.Fprintf(out, "%d,%dc%d\n", start1+1, end1, start2+1)
	default:
		fmt.Fprintf(out, "%d,%dc%d,%d\n", start1+1, end1, start2+1, end2)
	}

	for start1 < end1 {
		out.WriteString("< ")
		out.Write(data1[start1])
		out.WriteString("\n")
		start1++
	}
	out.WriteString("---\n")
	for start2 < end2 {
		out.WriteString("> ")
		out.Write(data2[start2])
		out.WriteString("\n")
		start2++
	}
}

func diff_text_insert(outfmt *OutputFormat, data1, data2 [][]byte, start1, end1, start2, end2 int) {
	diff_text_header(outfmt)
	if end2-start2 == 1 {
		fmt.Fprintf(out, "%da%d\n", start1, start2+1)
	} else {
		fmt.Fprintf(out, "%da%d,%d\n", start1, start2+1, end2)
	}

	for start2 < end2 {
		out.WriteString("> ")
		out.Write(data2[start2])
		out.WriteString("\n")
		start2++
	}
}

func diff_text_remove(outfmt *OutputFormat, data1, data2 [][]byte, start1, end1, start2, end2 int) {
	diff_text_header(outfmt)
	if end1-start1 == 1 {
		fmt.Fprintf(out, "%dd%d\n", start1+1, start2)
	} else {
		fmt.Fprintf(out, "%d,%dd%d\n", start1+1, end1, start2)
	}
	for start1 < end1 {
		out.WriteString("< ")
		out.Write(data1[start1])
		out.WriteString("\n")
		start1++
	}
}

func (file *Filedata) close_file() {
	if file.handle != nil {
		if file.is_mmapped && file.data != nil {
			syscall.Munmap(file.data)
		}
		file.handle.Close()
		file.handle = nil
	}
	file.data = nil
}

func is_space(b byte) bool {
	switch b {
	case ' ', '\t', '\v', '\f':
		return true
	}
	return false
}

func get_next_rune_nonspace(line []byte, i int) (rune, int) {
	b, size := utf8.DecodeRune(line[i:])
	i += size
	if !unicode.IsSpace(b) {
		return b, i
	}
	for i < len(line) {
		b, size := utf8.DecodeRune(line[i:])
		i += size
		if !unicode.IsSpace(b) {
			return b, i
		}
	}
	return 0, i
}

func get_next_rune_xspace(line []byte, i int) (rune, int) {
	b, size := utf8.DecodeRune(line[i:])
	i += size
	if !unicode.IsSpace(b) {
		return b, i
	}
	for i < len(line) {
		b, size := utf8.DecodeRune(line[i:])
		if !unicode.IsSpace(b) {
			return ' ', i
		}
		i += size
	}
	return ' ', i
}

func get_next_byte_nonspace(line []byte, i int) (byte, int) {
	b, i := line[i], i+1
	if !is_space(b) {
		return b, i
	}
	for i < len(line) {
		b, i = line[i], i+1
		if !is_space(b) {
			return b, i
		}
	}
	return 0, i
}

func get_next_byte_xspace(line []byte, i int) (byte, int) {
	b, i := line[i], i+1
	if !is_space(b) {
		return b, i
	}
	for i < len(line) {
		b = line[i]
		if !is_space(b) {
			return ' ', i
		}
		i = i + 1
	}
	return ' ', i
}

func compare_line_bytes(line1, line2 []byte) bool {
	len1, len2 := len(line1), len(line2)
	var i, j int
	var v1, v2 byte
	switch {
	case flag_cmp_ignore_all_space:
		for i < len1 && j < len2 {
			v1, i = get_next_byte_nonspace(line1, i)
			v2, j = get_next_byte_nonspace(line2, j)
			if flag_cmp_ignore_case {
				v1, v2 = to_lower_byte(v1), to_lower_byte(v2)
			}
			if v1 != v2 {
				return false
			}
		}
		if i < len1 || j < len2 {
			return false
		}

	case flag_cmp_ignore_space_change:
		for i < len1 && j < len2 {
			v1, i = get_next_byte_xspace(line1, i)
			v2, j = get_next_byte_xspace(line2, j)
			if flag_cmp_ignore_case {
				v1, v2 = to_lower_byte(v1), to_lower_byte(v2)
			}
			if v1 != v2 {
				return false
			}
		}
		if i < len1 || j < len2 {
			return false
		}

	case flag_cmp_ignore_case:
		if len1 != len2 {
			return false
		}
		for i < len1 && j < len2 {
			if to_lower_byte(line1[i]) != to_lower_byte(line2[j]) {
				return false
			}
			i, j = i+1, j+1
		}
		if i < len1 || j < len2 {
			return false
		}
	}
	return true
}

func compare_line_unicode(line1, line2 []byte) bool {
	len1, len2 := len(line1), len(line2)
	var i, j int
	var v1, v2 rune
	var size1, size2 int
	switch {
	case flag_cmp_ignore_all_space:
		for i < len1 && j < len2 {
			v1, i = get_next_rune_nonspace(line1, i)
			v2, j = get_next_rune_nonspace(line2, j)
			if flag_cmp_ignore_case {
				v1, v2 = unicode.ToLower(v1), unicode.ToLower(v2)
			}
			if v1 != v2 {
				return false
			}
		}
		if i < len1 || j < len2 {
			return false
		}

	case flag_cmp_ignore_space_change:
		for i < len1 && j < len2 {
			v1, i = get_next_rune_xspace(line1, i)
			v2, j = get_next_rune_xspace(line2, j)
			if flag_cmp_ignore_case {
				v1, v2 = unicode.ToLower(v1), unicode.ToLower(v2)
			}
			if v1 != v2 {
				return false
			}
		}
		if i < len1 || j < len2 {
			return false
		}

	case flag_cmp_ignore_case:
		if len1 != len2 {
			return false
		}
		for i < len1 && j < len2 {
			v1, size1 = utf8.DecodeRune(line1[i:])
			v2, size2 = utf8.DecodeRune(line2[j:])
			if unicode.ToLower(v1) != unicode.ToLower(v2) {
				return false
			}
			i, j = i+size1, j+size2
		}
		if i < len1 || j < len2 {
			return false
		}
	}
	return true
}

const fnv_offset32 = 2166136261
const fnv_prime32 = 16777619

func hash32(h uint32, b byte) uint32 {
	return (h ^ uint32(b)) * fnv_prime32
}

func compute_hash_exact(data []byte) uint32 {
	var h uint32 = fnv_offset32
	for _, v := range data {
		h = hash32(h, v)
	}
	return h
}

func compute_hash_bytes(line1 []byte) uint32 {
	var hash uint32 = fnv_offset32

	switch {
	case flag_cmp_ignore_all_space:
		for _, v1 := range line1 {
			if !is_space(v1) {
				if flag_cmp_ignore_case {
					v1 = to_lower_byte(v1)
				}
				hash = hash32(hash, v1)
			}
		}

	case flag_cmp_ignore_space_change:
		last_space := false
		for _, v1 := range line1 {
			if is_space(v1) {
				if last_space {
					continue
				}
				last_space = true
				v1 = ' '
			} else {
				last_space = false
				if flag_cmp_ignore_case {
					v1 = to_lower_byte(v1)
				}
			}
			hash = hash32(hash, v1)
		}

	case flag_cmp_ignore_case:
		for _, v1 := range line1 {
			v1 = to_lower_byte(v1)
			hash = hash32(hash, v1)
		}

	}
	return hash
}

func compute_hash_unicode(line1 []byte) uint32 {
	var hash uint32 = fnv_offset32

	i, len1 := 0, len(line1)

	switch {
	case flag_cmp_ignore_all_space:
		for i < len1 {
			v1, size := utf8.DecodeRune(line1[i:])
			i = i + size
			if !unicode.IsSpace(v1) {
				if flag_cmp_ignore_case {
					v1 = unicode.ToLower(v1)
				}
				for v1 != 0 {
					hash = hash32(hash, byte(v1))
					v1 = v1 >> 8
				}
			}
		}

	case flag_cmp_ignore_space_change:
		for i < len1 {
			v1, size := utf8.DecodeRune(line1[i:])
			i = i + size
			if unicode.IsSpace(v1) {
				for i < len1 {
					v2, size := utf8.DecodeRune(line1[i:])
					if !unicode.IsSpace(v2) {
						break
					}
					i += size
				}
				v1 = ' '
			}
			if flag_cmp_ignore_case {
				v1 = unicode.ToLower(v1)
			}
			for v1 != 0 {
				hash = hash32(hash, byte(v1))
				v1 = v1 >> 8
			}
		}

	case flag_cmp_ignore_case:
		for i < len1 {
			v1, size := utf8.DecodeRune(line1[i:])
			i = i + size
			v1 = unicode.ToLower(v1)
			for v1 != 0 {
				hash = hash32(hash, byte(v1))
				v1 = v1 >> 8
			}
		}
	}
	return hash
}

type EquivClass struct {
	line_id int
	line    []byte
	hash    uint32
	next    *EquivClass
}

type LinesData struct {
	ids    []int // Id's for each line, 
	zids   []int // list of ids with unmatched lines replaced by a single entry (and blank lines removed)
	zlines []int // Number of lines that represent each zids entry
}

//
// Compute id's that represent the original lines, these numeric id's are use for faster line comparison.
//
func find_equiv_lines(lines1, lines2 [][]byte) (*LinesData, *LinesData) {

	len1, len2 := len(lines1), len(lines2)
	info1, info2 := LinesData{}, LinesData{}
	info1.ids, info2.ids = make([]int, len1), make([]int, len2)

	// since we already have a hashing function, it's faster to use arrays than to use go's builtin map
	// Use bucket size that is power of 2
	buckets := 1 << 9
	for buckets < (len1+len2)*2 {
		buckets = buckets << 1
	}
	eqhash := make([]*EquivClass, buckets)

	// Use id=0 for blank lines. 
	// Later in report_changes(), do not report changes on chunks of lines with id=0
	if flag_cmp_ignore_blank_lines {
		blank := lines1[0][0:0] // blank line
		hashcode := compute_hash(blank)
		ihash := int(hashcode) & (buckets - 1)
		eqhash[ihash] = &EquivClass{ line_id: 0, line: blank, hash: hashcode }
	}

	// the unique id for identical lines, start with 1.
	next_id := 1

	// process both sets of lines
	for f := 0; f < 2; f++ {
		var lines [][]byte
		var ids []int

		if f == 0 {
			lines = lines1
			ids = info1.ids
		} else {
			lines = lines2
			ids = info2.ids
		}

		for i, ll := range lines {
			// find current line in eqhash
			hashcode := compute_hash(ll)
			ihash := int(hashcode) & (buckets - 1)
			eq := eqhash[ihash]
			switch {
			// not found in eqhash, create new entry
			case eq == nil:
				ids[i] = next_id
				eqhash[ihash] = &EquivClass{ line_id : next_id, line : ll, hash : hashcode }
				next_id++

			// found, and line is the same. reuse same id
			case eq.hash == hashcode && compare_line(ll, eq.line):
				ids[i] = eq.line_id

			default:
				// hash-collision. look through link-list for same match
				n := eq.next
				for ; n != nil; n = n.next {
					if n.hash == hashcode && compare_line(ll, n.line) {
						ids[i] = n.line_id
						break
					}
				}
				// new entry, link to start of linked-list
				if n == nil {
					eq.next = &EquivClass{ line_id: next_id, line: ll, hash: hashcode, next: eq.next }
					ids[i] = next_id
					next_id++
				}
			}
		}
	}

	compress_equiv_ids(&info1, &info2, next_id)

	return &info1, &info2
}

// Count the occurrances of each unique ids in both sets of lines, we will then know which lines are only present in one file, but not the other.
// Remove chunks of lines that do not appear in the other files, and replace with a single entry
// Return compressed lists of ids and a list indicating where are the chunk of lines being replaced
func compress_equiv_ids(lines1, lines2 *LinesData, next_id int) {

	count1, count2 := make([]int, next_id), make([]int, next_id)

	// count the number of occurrances of each id's
	for _, v := range lines1.ids {
		count1[v]++
	}
	for _, v := range lines2.ids {
		count2[v]++
	}

	// find identical line only appear exactly once in both sets
	len1, len2 :=  len(lines1.ids), len(lines2.ids)
	once2 := make([]int, next_id)
	for i, v := range lines2.ids {
		if count1[v] == 1 && count2[v] == 1 {
			once2[v] = i
		}
	}

	for  i := 0; i < len1; {
		v := lines1.ids[i]
		j := once2[v]
		if j == 0 {
			i++
			continue
		}
		iend, jend := i + 1, j + 1
		for iend < len1 && jend < len2 {
			if once2[lines1.ids[iend]] != jend {
				break
			}
			iend, jend = iend+1, jend+1
		}
//fmt.Fprintf(os.Stderr, "once: len=%d  %d <-> %d\n", iend - i, i, j)
		i = iend
	}

	// Go through all lines, replace chunk  lines that does not exists in the 
	// other set with a single entry and a new id).
	for f := 0; f < 2; f++ {
		var count_other, ids []int

		if f == 0 {
			ids = lines1.ids
			count_other = count2
		} else {
			ids = lines2.ids
			count_other = count1
		}

		// new slices for compressed ids and the number of lines each entry replaced
		zlines := make([]int, len(ids))
		zids := make([]int, len(ids))

		lastexclude := false
		n := 0
		for _, v := range ids {
			exclude := (count_other[v] == 0)
			if exclude && lastexclude {
				zlines[n-1]++
				zids[n-1] = -next_id
				next_id++
			} else if exclude {
				zlines[n]++
				zids[n] = -v
				n++
			} else {
				zlines[n]++
				zids[n] = v
				n++
			}
			lastexclude = exclude
		}

		// shrink the slice
		zids = zids[:n]
		zlines = zlines[:n]

		if f == 0 {
			lines1.zids = zids
			lines1.zlines = zlines
		} else {
			lines2.zids = zids
			lines2.zlines = zlines
		}

		//	fmt.Fprintf(os.Stderr, "   ids=%v\n", ids)
		//	fmt.Fprintf(os.Stderr, "  zids=%v\n", zids)
		//	fmt.Fprintf(os.Stderr, "zlines=%v\n", zlines)
	}

}

//
// Do the reverse of the compress_equiv_ids.
// zllines1 and zlines2 contains the 'extra' lines each entry represents.
//
func expand_change_list(info1, info2 *LinesData, zchange1, zchange2 []bool) ([]bool, []bool) {
	change1, change2 := make([]bool, len(info1.ids)), make([]bool, len(info2.ids))

	//fmt.Fprintf(os.Stderr, "zchange1=%v\n", zchange1)
	//fmt.Fprintf(os.Stderr, "zchange2=%v\n", zchange2)
	for f := 0; f < 2; f++ {
		var info *LinesData
		var change, zchange []bool

		if f == 0 {
			info = info1
			change = change1
			zchange = zchange1
		} else {
			info = info2
			change = change2
			zchange = zchange2
		}

		n := 0
		for i, m := range info.zlines {
			b := zchange[i]
			for j := 0; j < m; j, n = j+1, n+1 {
				change[n] = b
			}
		}
	}

	//fmt.Fprintf(os.Stderr, "change1=%v\n", change1)
	//fmt.Fprintf(os.Stderr, "change2=%v\n", change2)
	return change1, change2
}

func open_file(fname string, finfo os.FileInfo) *Filedata {

	file := &Filedata{ name: fname, info: finfo }

	var err error

	if file.info.Size() >= 1e9 {
		file.errormsg = MSG_FILE_TOO_BIG
		return file
	}

	if file.info.Size() <= 0 {
		return file
	}

	// open the file
	file.handle, err = os.Open(file.name)
	if err != nil {
		file.handle = nil
		file.errormsg = err.Error()
		return file
	}

	if file.info.Size() > MMAP_THRESHOLD {
		// map to file into memory, leave file open.
		file.data, err = syscall.Mmap(int(file.handle.Fd()), 0, int(file.info.Size()), syscall.PROT_READ, syscall.MAP_SHARED)
		if err != nil {
			file.handle.Close()
			file.handle = nil
			file.data = nil
			file.errormsg = err.Error()
			return file
		}
		file.is_mmapped = true
	} else {
		// read in the entire file
		buf := bytes.NewBuffer(make([]byte, 0, file.info.Size()+1))
		_, err = buf.ReadFrom(file.handle)
		if err != nil {
			file.errormsg = err.Error()
			return file
		}
		file.data = buf.Bytes()
		// close file
		file.handle.Close()
		file.handle = nil
	}

	// is binary if it has 'null' character
	if bytes.IndexByte(file.data[:min_int(BINARY_CHECK_SIZE, len(file.data))], 0) >= 0 {
		file.is_binary = true
		file.errormsg = MSG_FILE_IS_BINARY
		return file
	}
	return file
}

//
// split up data into text lines
//
func split_lines(data []byte) [][]byte {

	size := len(data)
	lines := make([][]byte, 0, size/64+10)
	previ := 0
	var lastb byte

	for i, b := range data {

		// accept dos, unix, mac newline
		if b == '\n' && lastb == '\r' {
			previ = i + 1
		} else if b == '\n' || b == '\r' {
			lines = append(lines, data[previ:i])
			previ = i + 1
		}
		lastb = b
	}

	// add last incomplete line (if required)
	if len(data) > previ {
		lines = append(lines, data[previ:len(data)])
	}

	return lines
}

//
// for sorting os.FileInfo by name
//
type FileInfoList []os.FileInfo

func (s FileInfoList) Len() int           { return len(s) }
func (s FileInfoList) Swap(i, j int)      { s[i], s[j] = s[j], s[i] }
func (s FileInfoList) Less(i, j int) bool { return s[i].Name() < s[j].Name() }

func read_sorted_dir(dirname string) ([]os.FileInfo, error) {

	dir, err := os.Open(dirname)
	if err != nil {
		return nil, err
	}

	all, err := dir.Readdir(-1)
	if err != nil {
		dir.Close()
		return nil, err
	}

	dir.Close()

	sort.Sort(FileInfoList(all))

	return all, nil
}

func diff_dirs(dirname1, dirname2 string, finfo1, finfo2 os.FileInfo) {

	dirname1 = strings.TrimRight(dirname1, PATH_SEPARATOR)
	dirname2 = strings.TrimRight(dirname2, PATH_SEPARATOR)

	dir1, err1 := read_sorted_dir(dirname1)
	dir2, err2 := read_sorted_dir(dirname2)

	if err1 != nil || err2 != nil {
		msg1, msg2 := "", ""
		if err1 != nil {
			msg1 = err1.Error()
		}
		if err2 != nil {
			msg2 = err2.Error()
		}
		output_diff_message(dirname1, dirname2, finfo1, finfo2, msg1, msg2, true)
		return
	}

	for _, dir_mode := range []bool{false, true} {
		i1, i2 := 0, 0
		for i1 < len(dir1) || i2 < len(dir2) {
			name1, name2 := "", ""
			if i1 < len(dir1) {
				name1 = dir1[i1].Name()
				if dir1[i1].IsDir() != dir_mode || strings.HasPrefix(name1, ".") {
					i1++
					continue
				}
			}
			if i2 < len(dir2) {
				name2 = dir2[i2].Name()
				if dir2[i2].IsDir() != dir_mode || strings.HasPrefix(name2, ".") {
					i2++
					continue
				}
			}

			if name1 == name2 {
				if dir1[i1].IsDir() != dir2[i2].IsDir() {
				} else if dir_mode {
					diff_dirs(dirname1+PATH_SEPARATOR+name1, dirname2+PATH_SEPARATOR+name2, dir1[i1], dir2[i2])
				} else {
					if flag_max_goroutines > 1 {
						queue_diff_file(dirname1+PATH_SEPARATOR+name1, dirname2+PATH_SEPARATOR+name2, dir1[i1], dir2[i2])
					} else {
						diff_file(dirname1+PATH_SEPARATOR+name1, dirname2+PATH_SEPARATOR+name2, dir1[i1], dir2[i2])
					}
				}
				i1++
				i2++
			} else if (i1 < len(dir1) && name1 < name2) || i2 >= len(dir2) {
				if dir_mode {
					output_diff_message(dirname1+PATH_SEPARATOR+name1, dirname2+PATH_SEPARATOR+name1, dir1[i1], nil, "", MSG_DIR_NOT_EXISTS, true)
				} else {
					if flag_suppress_missing_file {
						output_diff_message(dirname1+PATH_SEPARATOR+name1, dirname2+PATH_SEPARATOR+name1, dir1[i1], nil, "", MSG_FILE_NOT_EXISTS, true)
					} else {
						fdata := open_file(dirname1+PATH_SEPARATOR+name1, dir1[i1])
						output_diff_message_content(dirname1+PATH_SEPARATOR+name1, dirname2+PATH_SEPARATOR+name1, dir1[i1], nil, fdata.errormsg, MSG_FILE_NOT_EXISTS, fdata.data, nil, true)
						fdata.close_file()
					}
				}
				i1++
			} else if (i2 < len(dir2) && name2 < name1) || i1 >= len(dir1) {
				if dir_mode {
					output_diff_message(dirname1+PATH_SEPARATOR+name2, dirname2+PATH_SEPARATOR+name2, nil, dir2[i2], MSG_DIR_NOT_EXISTS, "", true)
				} else {
					if flag_suppress_missing_file {
						output_diff_message(dirname1+PATH_SEPARATOR+name2, dirname2+PATH_SEPARATOR+name2, nil, dir2[i2], MSG_FILE_NOT_EXISTS, "", true)
					} else {
						fdata := open_file(dirname2+PATH_SEPARATOR+name2, dir2[i2])
						output_diff_message_content(dirname1+PATH_SEPARATOR+name2, dirname2+PATH_SEPARATOR+name2, nil, dir2[i2], MSG_FILE_NOT_EXISTS, fdata.errormsg, nil, fdata.data, true)
						fdata.close_file()
					}
				}
				i2++
			} else {
				break
			}
		}
	}
}

func diff_file(filename1, filename2 string, finfo1, finfo2 os.FileInfo) {

	file1 := open_file(filename1, finfo1)
	file2 := open_file(filename2, finfo2)

	if file1.is_binary || file2.is_binary {

		var msg1, msg2 string

		switch {
		case file1.is_binary && file2.is_binary:
			// compare binary file
			if len(file1.data) != len(file2.data) || !bytes.Equal(file1.data, file2.data) {
				msg1, msg2 = MSG_BIN_FILE_DIFFERS, MSG_BIN_FILE_DIFFERS
			}

		case file1.is_binary:
			msg1 = file1.errormsg

		case file2.is_binary:
			msg2 = file1.errormsg
		}

		if msg1 != "" || msg2 != "" {
			output_diff_message(filename1, filename2, finfo1, finfo2, msg1, msg2, true)
		}
	} else if file1.errormsg != "" || file2.errormsg != "" {
		// display error messages
		output_diff_message(filename1, filename2, finfo1, finfo2, file1.errormsg, file2.errormsg, true)
	} else if bytes.Equal(file1.data, file2.data) {
		// files are equal
		if flag_show_identical_files {
			output_diff_message(filename1, filename2, finfo1, finfo2, MSG_FILE_IDENTICAL, MSG_FILE_IDENTICAL, false)
		}
	} else {
		lines1 := split_lines(file1.data)
		lines2 := split_lines(file2.data)

		// Compute equiv ids for each line.
		info1, info2 := find_equiv_lines(lines1, lines2)

		// run the diff algorithm
		change1, change2 := do_diff(info1.zids, info2.zids)

		if change1 != nil {

			// expand the change list, so that change array contains changes to actual lines
			change1, change2 = expand_change_list(info1, info2, change1, change2)

			// perform shift boundary
			shift_boundaries(info1.ids, change1, nil)
			shift_boundaries(info2.ids, change2, nil)

			action := DiffAction{}

			diffout := OutputFormat{
				name1:     filename1,
				name2:     filename2,
				fileinfo1: finfo1,
				fileinfo2: finfo2,
			}

			// Use function closures here as callbacks, it maps the line number from 
			// the comparison algorithm to the actual line lines
			if flag_output_as_text {
				// for output in text format
				action.diff_insert = func(start1, end1, start2, end2 int) {
					diff_text_insert(&diffout, lines1, lines2, start1, end1, start2, end2)
				}

				action.diff_modify = func(start1, end1, start2, end2 int) {
					diff_text_modify(&diffout, lines1, lines2, start1, end1, start2, end2)
				}

				action.diff_remove = func(start1, end1, start2, end2 int) {
					diff_text_remove(&diffout, lines1, lines2, start1, end1, start2, end2)
				}
			} else {
				// for output in html format
				action.diff_insert = func(start1, end1, start2, end2 int) {
					diff_html_insert(&diffout, lines1, lines2, start1, end1, start2, end2)
				}

				action.diff_modify = func(start1, end1, start2, end2 int) {
					diff_html_modify(&diffout, lines1, lines2, start1, end1, start2, end2)
				}

				action.diff_remove = func(start1, end1, start2, end2 int) {
					diff_html_remove(&diffout, lines1, lines2, start1, end1, start2, end2)
				}
			}

			report_changes(&action, info1.ids, info2.ids, change1, change2)

			if flag_output_as_text {

				if diffout.header_printed {
					diffout.header_printed = false
					out_release_lock()
				}

			} else {

				html_add_context_lines(&diffout, lines1, lines2, len(lines1), len(lines2))
				html_add_block(&diffout)

				if diffout.header_printed {
					out.WriteString("</table><br>\n")
					diffout.header_printed = false
					out_release_lock()
				}
			}

		} else if flag_show_identical_files {
			// report on identical file if required
			output_diff_message(filename1, filename2, finfo1, finfo2, MSG_FILE_IDENTICAL, MSG_FILE_IDENTICAL, false)
		}
	}

	file1.close_file()
	file2.close_file()
}

func abs_int(a int) int {
	if a < 0 {
		return -a
	}
	return a
}

func max_int(a, b int) int {
	if a < b {
		return b
	}
	return a
}

func min_int(a, b int) int {
	if a < b {
		return a
	}
	return b
}

//
// An O(ND) Difference Algorithm: Find middle snake
//
func algorithm_sms(data1, data2 []int, v []int, start_d int) (int, int) {

	end1, end2 := len(data1), len(data2)
	max := end1 + end2 + 1
	up_k := end1 - end2
	odd := (up_k & 1) != 0
	down_off := max
	up_off := max - up_k + (max * 2) + 2
	var x, y, z int

	v[down_off+1] = 0
	v[up_off+up_k-1] = end1

	for d := start_d; true; d++ {
		for k := -d; k <= d; k += 2 {
			x = v[down_off+k+1]
			if k > -d {
				z = v[down_off+k-1] + 1
				if (k == d) || (z > x) {
					x = z
				}
			}
			y = x - k
			for (x < end1) && (y < end2) && data1[x] == data2[y] {
				x, y = x+1, y+1
			}
			if odd && (up_k-d < k) && (k < up_k+d) && v[up_off+k] <= x {
				return x, x - k
			}
			v[down_off+k] = x
		}

		for k := up_k - d; k <= up_k+d; k += 2 {
			x = v[up_off+k-1]
			if k < up_k+d {
				z = v[up_off+k+1] - 1
				if (k == up_k-d) || (z < x) {
					x = z
				}
			}
			y = x - k
			for (x > 0) && (y > 0) && data1[x-1] == data2[y-1] {
				x, y = x-1, y-1
			}
			if !odd && (-d <= k) && (k <= d) && x <= v[down_off+k] {
				return x, x - k
			}
			v[up_off+k] = x
		}
	}
	return 0, 0 // should not reach here
}

//
// An O(ND) Difference Algorithm: Find LCS
//
func algorithm_lcs(data1, data2 []int, change1, change2 []bool, v []int) bool {

	start1, end1 := 0, len(data1)
	start2, end2 := 0, len(data2)
	changed := false

	// matches found at start and end of list
	for start1 < end1 && start2 < end2 && data1[start1] == data2[start2] {
		start1, start2 = start1+1, start2+1
	}
	for start1 < end1 && start2 < end2 && data1[end1-1] == data2[end2-1] {
		end1, end2 = end1-1, end2-1
	}

	switch {
	case start1 == end1:
		// mark remaining data2 as 'inserted'
		for start2 < end2 {
			change2[start2], start2 = true, start2+1
			changed = true
		}

	case start2 == end2:
		// mark remaining data1 as 'deleted'
		for start1 < end1 {
			change1[start1], start1 = true, start1+1
			changed = true
		}

	default:
		// Find a point of correspondence in the middle of the vectors.
		mid1, mid2 := algorithm_sms(data1[start1:end1], data2[start2:end2], v, 0)
		mid1, mid2 = mid1+start1, mid2+start2

		// Use the partitions to split this problem into subproblems.
		r1 := algorithm_lcs(data1[start1:mid1], data2[start2:mid2], change1[start1:mid1], change2[start2:mid2], v)
		r2 := algorithm_lcs(data1[mid1:end1], data2[mid2:end2], change1[mid1:end1], change2[mid2:end2], v)
		changed = changed || r1 || r2
	}
	return changed
}

// Perform the shift
func do_shift_boundary(start, end, offset int, change []bool) {
	if offset < 0 {
		offset = -offset
		for offset > 0 {
			start, end, offset = start-1, end-1, offset-1
			change[start], change[end] = true, false
		}
	} else {
		for offset > 0 {
			change[start], change[end] = false, true
			start, end, offset = start+1, end+1, offset-1
		}
	}
}

// Determine if the changes starting at 'pos' can be shifted 'up' or 'down'
func find_shift_boundary(start int, data []int, change []bool) (int, int, int, bool, bool) {
	end, dlen := start+1, len(data)
	up, down := 0, 0

	// Find the end of this chunk of changes
	for end < dlen && change[end] {
		end++
	}

	for start-up-1 >= 0 && !change[start-up-1] && data[start-up-1] == data[end-up-1] {
		up = up + 1
	}

	for end+down < dlen && !change[end+down] && data[end+down] == data[start+down] {
		down = down + 1
	}

	// has changes been shifted to start/end of list or joined with previous/next change
	up_join := (start-up == 0) || change[start-up-1]
	down_join := (end+down == dlen) || change[end+down]

	return end, up, down, up_join, down_join
}

func rune_edge_score(r rune) int {

	switch r {
	case ' ', '\t', '\v', '\f':
		return 100

	case '<', '>', '(', ')', '[', ']', '\'', '"':
		return 40
	}

	return 0
}

// scoring character boundary, for finding a change chunk that is easier to read
func rune_bouundary_score(r1, r2 int) int {

	s1 := rune_edge_score(rune(r1))
	s2 := rune_edge_score(rune(r2))

	return s1 + s2
}

func shift_boundaries(data []int, change []bool, boundary_score func(int, int) int) {

	start, clen := 0, len(change)

	for start < clen {
		// find the next chunk of changes
		for start < clen && !change[start] {
			start++
		}
		if start >= clen {
			break
		}

		end, up, down, up_join, down_join := find_shift_boundary(start, data, change)
		switch {
		case up > 0 && up_join:
			// shift up, merged with previous chunk of changes
			do_shift_boundary(start, end, -up, change)
			for start-1 >= 0 && change[start-1] {
				start--
			}

		case down > 0 && down_join:
			// shift down, merged with next chunk of changes
			do_shift_boundary(start, end, down, change)
			if end+down < clen {
				start = start + down
			} else {
				start = clen
			}

		default:
			// Only perform shifts when there is a boundary score
			if (up > 0 || down > 0) && boundary_score != nil {
				offset, best_score := 0, -1
				for i := -up; i <= down; i++ {
					if i != 0 {
						score := boundary_score(data[start+i], data[end+i-1])
						if offset == 0 || score > best_score {
							offset, best_score = i, score
						}
					}
				}
				if offset != 0 {
					do_shift_boundary(start, end, offset, change)
				}
				if offset < 0 {
					start = end
				} else {
					start = end + offset
				}
			} else {
				start = end
			}
		}
	}
}

// Wait for all jobs to finish
func job_queue_finish() {
	if flag_max_goroutines > 1 {
		job_wait.Wait()
	}
}

// Initialise job queues
func job_queue_init() {

	if flag_max_goroutines > 1 {

		if flag_max_goroutines > runtime.GOMAXPROCS(-1) {
			runtime.GOMAXPROCS(flag_max_goroutines)
		}

		// create async job queue channel
		job_queue = make(chan JobQueue, 1)

		// start up goroutines, to handle file comparison
		for i := 0; i < flag_max_goroutines; i++ {
			go func() {
				for job := range job_queue {
					diff_file(job.name1, job.name2, job.info1, job.info2)
					job_wait.Done()
				}
			}()
		}
	}
}

// Queue file comparison
func queue_diff_file(fname1, fname2 string, finfo1, finfo2 os.FileInfo) {
	job := JobQueue{
		name1: fname1,
		name2: fname2,
		info1: finfo1,
		info2: finfo2,
	}

	job_wait.Add(1)
	job_queue <- job
}

// Acquire Mutext lock on output stream
func out_acquire_lock() {
	if flag_max_goroutines > 1 {
		out_lock.Lock()
	}
}

// Release Mutext lock on output stream
func out_release_lock() {
	if flag_max_goroutines > 1 {
		out_lock.Unlock()
	}
}
