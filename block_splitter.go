package brotli

/* Copyright 2013 Google Inc. All Rights Reserved.

   Distributed under MIT license.
   See file LICENSE for detail or copy at https://opensource.org/licenses/MIT
*/

/* Build per-context histograms of literals, commands and distance codes. */
/* Copyright 2013 Google Inc. All Rights Reserved.

   Distributed under MIT license.
   See file LICENSE for detail or copy at https://opensource.org/licenses/MIT
*/

/* Models the histograms of literals, commands and distance codes. */
/* Copyright 2013 Google Inc. All Rights Reserved.

   Distributed under MIT license.
   See file LICENSE for detail or copy at https://opensource.org/licenses/MIT
*/

/* Block split point selection utilities. */
type BlockSplit struct {
	num_types          uint
	num_blocks         uint
	types              []byte
	lengths            []uint32
	types_alloc_size   uint
	lengths_alloc_size uint
}

var kMaxLiteralHistograms uint = 100

var kMaxCommandHistograms uint = 50

var kLiteralBlockSwitchCost float64 = 28.1

var kCommandBlockSwitchCost float64 = 13.5

var kDistanceBlockSwitchCost float64 = 14.6

var kLiteralStrideLength uint = 70

var kCommandStrideLength uint = 40

var kSymbolsPerLiteralHistogram uint = 544

var kSymbolsPerCommandHistogram uint = 530

var kSymbolsPerDistanceHistogram uint = 544

var kMinLengthForBlockSplitting uint = 128

var kIterMulForRefining uint = 2

var kMinItersForRefining uint = 100

func CountLiterals(cmds []Command, num_commands uint) uint {
	var total_length uint = 0
	/* Count how many we have. */

	var i uint
	for i = 0; i < num_commands; i++ {
		total_length += uint(cmds[i].insert_len_)
	}

	return total_length
}

func CopyLiteralsToByteArray(cmds []Command, num_commands uint, data []byte, offset uint, mask uint, literals []byte) {
	var pos uint = 0
	var from_pos uint = offset & mask
	var i uint
	for i = 0; i < num_commands; i++ {
		var insert_len uint = uint(cmds[i].insert_len_)
		if from_pos+insert_len > mask {
			var head_size uint = mask + 1 - from_pos
			copy(literals[pos:], data[from_pos:][:head_size])
			from_pos = 0
			pos += head_size
			insert_len -= head_size
		}

		if insert_len > 0 {
			copy(literals[pos:], data[from_pos:][:insert_len])
			pos += insert_len
		}

		from_pos = uint((uint32(from_pos+insert_len) + CommandCopyLen(&cmds[i])) & uint32(mask))
	}
}

func MyRand(seed *uint32) uint32 {
	/* Initial seed should be 7. In this case, loop length is (1 << 29). */
	*seed *= 16807

	return *seed
}

func BitCost(count uint) float64 {
	if count == 0 {
		return -2.0
	} else {
		return FastLog2(count)
	}
}

const HISTOGRAMS_PER_BATCH = 64

const CLUSTERS_PER_BATCH = 16

func BrotliInitBlockSplit(self *BlockSplit) {
	self.num_types = 0
	self.num_blocks = 0
	self.types = nil
	self.lengths = nil
	self.types_alloc_size = 0
	self.lengths_alloc_size = 0
}

func BrotliDestroyBlockSplit(self *BlockSplit) {
	self.types = nil
	self.lengths = nil
}

func BrotliSplitBlock(cmds []Command, num_commands uint, data []byte, pos uint, mask uint, params *BrotliEncoderParams, literal_split *BlockSplit, insert_and_copy_split *BlockSplit, dist_split *BlockSplit) {
	{
		var literals_count uint = CountLiterals(cmds, num_commands)
		var literals []byte = make([]byte, literals_count)

		/* Create a continuous array of literals. */
		CopyLiteralsToByteArray(cmds, num_commands, data, pos, mask, literals)

		/* Create the block split on the array of literals.
		   Literal histograms have alphabet size 256. */
		SplitByteVectorLiteral(literals, literals_count, kSymbolsPerLiteralHistogram, kMaxLiteralHistograms, kLiteralStrideLength, kLiteralBlockSwitchCost, params, literal_split)

		literals = nil
	}
	{
		var insert_and_copy_codes []uint16 = make([]uint16, num_commands)
		/* Compute prefix codes for commands. */

		var i uint
		for i = 0; i < num_commands; i++ {
			insert_and_copy_codes[i] = cmds[i].cmd_prefix_
		}

		/* Create the block split on the array of command prefixes. */
		SplitByteVectorCommand(insert_and_copy_codes, num_commands, kSymbolsPerCommandHistogram, kMaxCommandHistograms, kCommandStrideLength, kCommandBlockSwitchCost, params, insert_and_copy_split)

		/* TODO: reuse for distances? */

		insert_and_copy_codes = nil
	}
	{
		var distance_prefixes []uint16 = make([]uint16, num_commands)
		var j uint = 0
		/* Create a continuous array of distance prefixes. */

		var i uint
		for i = 0; i < num_commands; i++ {
			var cmd *Command = &cmds[i]
			if CommandCopyLen(cmd) != 0 && cmd.cmd_prefix_ >= 128 {
				distance_prefixes[j] = cmd.dist_prefix_ & 0x3FF
				j++
			}
		}

		/* Create the block split on the array of distance prefixes. */
		SplitByteVectorDistance(distance_prefixes, j, kSymbolsPerDistanceHistogram, kMaxCommandHistograms, kCommandStrideLength, kDistanceBlockSwitchCost, params, dist_split)

		distance_prefixes = nil
	}
}