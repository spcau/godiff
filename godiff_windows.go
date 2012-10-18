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
package main

import (
	"os"
	"syscall"
	"unsafe"
)

// Implement mmap for windows
func map_file(file *os.File, offset, size int) ([]byte, error) {
	// call the windows function
	addr, err := syscall.MapViewOfFile(syscall.Handle(file.Fd()), syscall.FILE_MAP_READ, uint32(offset>>32), uint32(offset), uintptr(size))

	if err != nil {
		return nil, err
	}

	// Slice memory layout
	sl := struct {
		addr uintptr
		len  int
		cap  int
	}{addr, size, size}

	// Use unsafe to turn sl into a []byte.
	bp := *(*[]byte)(unsafe.Pointer(&sl))

	return bp, err
}

// Implement munmap for windows
func unmap_file(data []byte) error {

	// Use unsafe to get the buffer address
	addr := uintptr(unsafe.Pointer(&data[0]))

	// call the windows function
	return syscall.UnmapViewOfFile(addr)
}
