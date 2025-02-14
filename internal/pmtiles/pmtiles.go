// Copyright 2021 Protomaps LLC
//
// Redistribution and use in source and binary forms, with or without modification, are permitted provided that the following conditions are met:
//
// 1. Redistributions of source code must retain the above copyright notice, this list of conditions and the following disclaimer.
//
// 2. Redistributions in binary form must reproduce the above copyright notice, this list of conditions and the following disclaimer in the documentation and/or other materials provided with the distribution.
//
// 3. Neither the name of the copyright holder nor the names of its contributors may be used to endorse or promote products derived from this software without specific prior written permission.
//
// THIS SOFTWARE IS PROVIDED BY THE COPYRIGHT HOLDERS AND CONTRIBUTORS "AS IS" AND ANY EXPRESS OR IMPLIED WARRANTIES, INCLUDING, BUT NOT LIMITED TO, THE IMPLIED WARRANTIES OF MERCHANTABILITY AND FITNESS FOR A PARTICULAR PURPOSE ARE DISCLAIMED. IN NO EVENT SHALL THE COPYRIGHT HOLDER OR CONTRIBUTORS BE LIABLE FOR ANY DIRECT, INDIRECT, INCIDENTAL, SPECIAL, EXEMPLARY, OR CONSEQUENTIAL DAMAGES (INCLUDING, BUT NOT LIMITED TO, PROCUREMENT OF SUBSTITUTE GOODS OR SERVICES; LOSS OF USE, DATA, OR PROFITS; OR BUSINESS INTERRUPTION) HOWEVER CAUSED AND ON ANY THEORY OF LIABILITY, WHETHER IN CONTRACT, STRICT LIABILITY, OR TORT (INCLUDING NEGLIGENCE OR OTHERWISE) ARISING IN ANY WAY OUT OF THE USE OF THIS SOFTWARE, EVEN IF ADVISED OF THE POSSIBILITY OF SUCH DAMAGE.
package pmtiles

// ZxyToID converts (Z,X,Y) tile coordinates to a Hilbert TileID.
func ZxyToID(z uint8, x uint32, y uint32) uint64 {
	var acc uint64
	var tz uint8
	for ; tz < z; tz++ {
		acc += (0x1 << tz) * (0x1 << tz)
	}
	var n uint64 = 1 << z
	var rx uint64
	var ry uint64
	var d uint64
	tx := uint64(x)
	ty := uint64(y)
	for s := n / 2; s > 0; s /= 2 {
		if tx&s > 0 {
			rx = 1
		} else {
			rx = 0
		}
		if ty&s > 0 {
			ry = 1
		} else {
			ry = 0
		}
		d += s * s * ((3 * rx) ^ ry)
		rotate(s, &tx, &ty, rx, ry)
	}
	return acc + d
}

func rotate(n uint64, x *uint64, y *uint64, rx uint64, ry uint64) {
	if ry == 0 {
		if rx == 1 {
			*x = n - 1 - *x
			*y = n - 1 - *y
		}
		*x, *y = *y, *x
	}
}

// IDToZxy converts a Hilbert TileID to (Z,X,Y) tile coordinates.
func IDToZxy(i uint64) (uint8, uint32, uint32) {
	var acc uint64
	var z uint8
	for {
		var numTiles uint64
		numTiles = (1 << z) * (1 << z)
		if acc+numTiles > i {
			return tOnLevel(z, i-acc)
		}
		acc += numTiles
		z++
	}
}

func tOnLevel(z uint8, pos uint64) (uint8, uint32, uint32) {
	var n uint64 = 1 << z
	rx, ry, t := pos, pos, pos
	var tx uint64
	var ty uint64
	var s uint64
	for s = 1; s < n; s *= 2 {
		rx = 1 & (t / 2)
		ry = 1 & (t ^ rx)
		rotate(s, &tx, &ty, rx, ry)
		tx += s * rx
		ty += s * ry
		t /= 4
	}
	return uint8(z), uint32(tx), uint32(ty)
}
