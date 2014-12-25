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

// +build linux freebsd darwin

package main

import (
	"os"
	"syscall"
)

const has_mmap = true

func map_file(file *os.File, offset int64, size int) ([]byte, error) {
	data, err := syscall.Mmap(int(file.Fd()), offset, size, syscall.PROT_READ, syscall.MAP_SHARED)
	return data, err
}

func unmap_file(data []byte) error {
	return syscall.Munmap(data)
}
