package brotli

/* Copyright 2014 Google Inc. All Rights Reserved.

   Distributed under MIT license.
   See file LICENSE for detail or copy at https://opensource.org/licenses/MIT
*/

/* Brotli bit stream functions to support the low level format. There are no
   compression algorithms here, just the right ordering of bits to match the
   specs. */
/* Copyright 2014 Google Inc. All Rights Reserved.

   Distributed under MIT license.
   See file LICENSE for detail or copy at https://opensource.org/licenses/MIT
*/

/* Functions to convert brotli-related data structures into the
   brotli bit stream. The functions here operate under
   assumption that there is enough space in the storage, i.e., there are
   no out-of-range checks anywhere.

   These functions do bit addressing into a byte array. The byte array
   is called "storage" and the index to the bit is called storage_ix
   in function arguments. */
/* Copyright 2015 Google Inc. All Rights Reserved.

   Distributed under MIT license.
   See file LICENSE for detail or copy at https://opensource.org/licenses/MIT
*/

/* Algorithms for distributing the literals and commands of a metablock between
   block types and contexts. */
type MetaBlockSplit struct {
	literal_split             BlockSplit
	command_split             BlockSplit
	distance_split            BlockSplit
	literal_context_map       []uint32
	literal_context_map_size  uint
	distance_context_map      []uint32
	distance_context_map_size uint
	literal_histograms        []HistogramLiteral
	literal_histograms_size   uint
	command_histograms        []HistogramCommand
	command_histograms_size   uint
	distance_histograms       []HistogramDistance
	distance_histograms_size  uint
}

func InitMetaBlockSplit(mb *MetaBlockSplit) {
	BrotliInitBlockSplit(&mb.literal_split)
	BrotliInitBlockSplit(&mb.command_split)
	BrotliInitBlockSplit(&mb.distance_split)
	mb.literal_context_map = nil
	mb.literal_context_map_size = 0
	mb.distance_context_map = nil
	mb.distance_context_map_size = 0
	mb.literal_histograms = nil
	mb.literal_histograms_size = 0
	mb.command_histograms = nil
	mb.command_histograms_size = 0
	mb.distance_histograms = nil
	mb.distance_histograms_size = 0
}

func DestroyMetaBlockSplit(mb *MetaBlockSplit) {
	BrotliDestroyBlockSplit(&mb.literal_split)
	BrotliDestroyBlockSplit(&mb.command_split)
	BrotliDestroyBlockSplit(&mb.distance_split)
	mb.literal_context_map = nil
	mb.distance_context_map = nil
	mb.literal_histograms = nil
	mb.command_histograms = nil
	mb.distance_histograms = nil
}

/* Copyright 2015 Google Inc. All Rights Reserved.

   Distributed under MIT license.
   See file LICENSE for detail or copy at https://opensource.org/licenses/MIT
*/

/* Algorithms for distributing the literals and commands of a metablock between
   block types and contexts. */
func BrotliInitDistanceParams(params *BrotliEncoderParams, npostfix uint32, ndirect uint32) {
	var dist_params *BrotliDistanceParams = &params.dist
	var alphabet_size uint32
	var max_distance uint32

	dist_params.distance_postfix_bits = npostfix
	dist_params.num_direct_distance_codes = ndirect

	alphabet_size = uint32(BROTLI_DISTANCE_ALPHABET_SIZE(uint(npostfix), uint(ndirect), BROTLI_MAX_DISTANCE_BITS))
	max_distance = ndirect + (1 << (BROTLI_MAX_DISTANCE_BITS + npostfix + 2)) - (1 << (npostfix + 2))

	if params.large_window {
		var bound = [BROTLI_MAX_NPOSTFIX + 1]uint32{0, 4, 12, 28}
		var postfix uint32 = 1 << npostfix
		alphabet_size = uint32(BROTLI_DISTANCE_ALPHABET_SIZE(uint(npostfix), uint(ndirect), BROTLI_LARGE_MAX_DISTANCE_BITS))

		/* The maximum distance is set so that no distance symbol used can encode
		   a distance larger than BROTLI_MAX_ALLOWED_DISTANCE with all
		   its extra bits set. */
		if ndirect < bound[npostfix] {
			max_distance = BROTLI_MAX_ALLOWED_DISTANCE - (bound[npostfix] - ndirect)
		} else if ndirect >= bound[npostfix]+postfix {
			max_distance = (3 << 29) - 4 + (ndirect - bound[npostfix])
		} else {
			max_distance = BROTLI_MAX_ALLOWED_DISTANCE
		}
	}

	dist_params.alphabet_size = alphabet_size
	dist_params.max_distance = uint(max_distance)
}

func RecomputeDistancePrefixes(cmds []Command, num_commands uint, orig_params *BrotliDistanceParams, new_params *BrotliDistanceParams) {
	var i uint

	if orig_params.distance_postfix_bits == new_params.distance_postfix_bits && orig_params.num_direct_distance_codes == new_params.num_direct_distance_codes {
		return
	}

	for i = 0; i < num_commands; i++ {
		var cmd *Command = &cmds[i]
		if CommandCopyLen(cmd) != 0 && cmd.cmd_prefix_ >= 128 {
			PrefixEncodeCopyDistance(uint(CommandRestoreDistanceCode(cmd, orig_params)), uint(new_params.num_direct_distance_codes), uint(new_params.distance_postfix_bits), &cmd.dist_prefix_, &cmd.dist_extra_)
		}
	}
}

func ComputeDistanceCost(cmds []Command, num_commands uint, orig_params *BrotliDistanceParams, new_params *BrotliDistanceParams, cost *float64) bool {
	var i uint
	var equal_params bool = false
	var dist_prefix uint16
	var dist_extra uint32
	var extra_bits float64 = 0.0
	var histo HistogramDistance
	HistogramClearDistance(&histo)

	if orig_params.distance_postfix_bits == new_params.distance_postfix_bits && orig_params.num_direct_distance_codes == new_params.num_direct_distance_codes {
		equal_params = true
	}

	for i = 0; i < num_commands; i++ {
		var cmd *Command = &cmds[i]
		if CommandCopyLen(cmd) != 0 && cmd.cmd_prefix_ >= 128 {
			if equal_params {
				dist_prefix = cmd.dist_prefix_
			} else {
				var distance uint32 = CommandRestoreDistanceCode(cmd, orig_params)
				if distance > uint32(new_params.max_distance) {
					return false
				}

				PrefixEncodeCopyDistance(uint(distance), uint(new_params.num_direct_distance_codes), uint(new_params.distance_postfix_bits), &dist_prefix, &dist_extra)
			}

			HistogramAddDistance(&histo, uint(dist_prefix)&0x3FF)
			extra_bits += float64(dist_prefix >> 10)
		}
	}

	*cost = BrotliPopulationCostDistance(&histo) + extra_bits
	return true
}

var BrotliBuildMetaBlock_kMaxNumberOfHistograms uint = 256

func BrotliBuildMetaBlock(ringbuffer []byte, pos uint, mask uint, params *BrotliEncoderParams, prev_byte byte, prev_byte2 byte, cmds []Command, num_commands uint, literal_context_mode int, mb *MetaBlockSplit) {
	var distance_histograms []HistogramDistance
	var literal_histograms []HistogramLiteral
	var literal_context_modes []int = nil
	var literal_histograms_size uint
	var distance_histograms_size uint
	var i uint
	var literal_context_multiplier uint = 1
	var npostfix uint32
	var ndirect_msb uint32 = 0
	var check_orig bool = true
	var best_dist_cost float64 = 1e99
	var orig_params BrotliEncoderParams = *params
	/* Histogram ids need to fit in one byte. */

	var new_params BrotliEncoderParams = *params

	for npostfix = 0; npostfix <= BROTLI_MAX_NPOSTFIX; npostfix++ {
		for ; ndirect_msb < 16; ndirect_msb++ {
			var ndirect uint32 = ndirect_msb << npostfix
			var skip bool
			var dist_cost float64
			BrotliInitDistanceParams(&new_params, npostfix, ndirect)
			if npostfix == orig_params.dist.distance_postfix_bits && ndirect == orig_params.dist.num_direct_distance_codes {
				check_orig = false
			}

			skip = !ComputeDistanceCost(cmds, num_commands, &orig_params.dist, &new_params.dist, &dist_cost)
			if skip || (dist_cost > best_dist_cost) {
				break
			}

			best_dist_cost = dist_cost
			params.dist = new_params.dist
		}

		if ndirect_msb > 0 {
			ndirect_msb--
		}
		ndirect_msb /= 2
	}

	if check_orig {
		var dist_cost float64
		ComputeDistanceCost(cmds, num_commands, &orig_params.dist, &orig_params.dist, &dist_cost)
		if dist_cost < best_dist_cost {
			/* NB: currently unused; uncomment when more param tuning is added. */
			/* best_dist_cost = dist_cost; */
			params.dist = orig_params.dist
		}
	}

	RecomputeDistancePrefixes(cmds, num_commands, &orig_params.dist, &params.dist)

	BrotliSplitBlock(cmds, num_commands, ringbuffer, pos, mask, params, &mb.literal_split, &mb.command_split, &mb.distance_split)

	if !params.disable_literal_context_modeling {
		literal_context_multiplier = 1 << BROTLI_LITERAL_CONTEXT_BITS
		literal_context_modes = make([]int, (mb.literal_split.num_types))
		for i = 0; i < mb.literal_split.num_types; i++ {
			literal_context_modes[i] = literal_context_mode
		}
	}

	literal_histograms_size = mb.literal_split.num_types * literal_context_multiplier
	literal_histograms = make([]HistogramLiteral, literal_histograms_size)
	ClearHistogramsLiteral(literal_histograms, literal_histograms_size)

	distance_histograms_size = mb.distance_split.num_types << BROTLI_DISTANCE_CONTEXT_BITS
	distance_histograms = make([]HistogramDistance, distance_histograms_size)
	ClearHistogramsDistance(distance_histograms, distance_histograms_size)

	assert(mb.command_histograms == nil)
	mb.command_histograms_size = mb.command_split.num_types
	mb.command_histograms = make([]HistogramCommand, (mb.command_histograms_size))
	ClearHistogramsCommand(mb.command_histograms, mb.command_histograms_size)

	BrotliBuildHistogramsWithContext(cmds, num_commands, &mb.literal_split, &mb.command_split, &mb.distance_split, ringbuffer, pos, mask, prev_byte, prev_byte2, literal_context_modes, literal_histograms, mb.command_histograms, distance_histograms)
	literal_context_modes = nil

	assert(mb.literal_context_map == nil)
	mb.literal_context_map_size = mb.literal_split.num_types << BROTLI_LITERAL_CONTEXT_BITS
	mb.literal_context_map = make([]uint32, (mb.literal_context_map_size))

	assert(mb.literal_histograms == nil)
	mb.literal_histograms_size = mb.literal_context_map_size
	mb.literal_histograms = make([]HistogramLiteral, (mb.literal_histograms_size))

	BrotliClusterHistogramsLiteral(literal_histograms, literal_histograms_size, BrotliBuildMetaBlock_kMaxNumberOfHistograms, mb.literal_histograms, &mb.literal_histograms_size, mb.literal_context_map)
	literal_histograms = nil

	if params.disable_literal_context_modeling {
		/* Distribute assignment to all contexts. */
		for i = mb.literal_split.num_types; i != 0; {
			var j uint = 0
			i--
			for ; j < 1<<BROTLI_LITERAL_CONTEXT_BITS; j++ {
				mb.literal_context_map[(i<<BROTLI_LITERAL_CONTEXT_BITS)+j] = mb.literal_context_map[i]
			}
		}
	}

	assert(mb.distance_context_map == nil)
	mb.distance_context_map_size = mb.distance_split.num_types << BROTLI_DISTANCE_CONTEXT_BITS
	mb.distance_context_map = make([]uint32, (mb.distance_context_map_size))

	assert(mb.distance_histograms == nil)
	mb.distance_histograms_size = mb.distance_context_map_size
	mb.distance_histograms = make([]HistogramDistance, (mb.distance_histograms_size))

	BrotliClusterHistogramsDistance(distance_histograms, mb.distance_context_map_size, BrotliBuildMetaBlock_kMaxNumberOfHistograms, mb.distance_histograms, &mb.distance_histograms_size, mb.distance_context_map)
	distance_histograms = nil
}

const BROTLI_MAX_STATIC_CONTEXTS = 13

/* Greedy block splitter for one block category (literal, command or distance).
   Gathers histograms for all context buckets. */
type ContextBlockSplitter struct {
	alphabet_size_     uint
	num_contexts_      uint
	max_block_types_   uint
	min_block_size_    uint
	split_threshold_   float64
	num_blocks_        uint
	split_             *BlockSplit
	histograms_        []HistogramLiteral
	histograms_size_   *uint
	target_block_size_ uint
	block_size_        uint
	curr_histogram_ix_ uint
	last_histogram_ix_ [2]uint
	last_entropy_      [2 * BROTLI_MAX_STATIC_CONTEXTS]float64
	merge_last_count_  uint
}

func InitContextBlockSplitter(self *ContextBlockSplitter, alphabet_size uint, num_contexts uint, min_block_size uint, split_threshold float64, num_symbols uint, split *BlockSplit, histograms *[]HistogramLiteral, histograms_size *uint) {
	var max_num_blocks uint = num_symbols/min_block_size + 1
	var max_num_types uint
	assert(num_contexts <= BROTLI_MAX_STATIC_CONTEXTS)

	self.alphabet_size_ = alphabet_size
	self.num_contexts_ = num_contexts
	self.max_block_types_ = BROTLI_MAX_NUMBER_OF_BLOCK_TYPES / num_contexts
	self.min_block_size_ = min_block_size
	self.split_threshold_ = split_threshold
	self.num_blocks_ = 0
	self.split_ = split
	self.histograms_size_ = histograms_size
	self.target_block_size_ = min_block_size
	self.block_size_ = 0
	self.curr_histogram_ix_ = 0
	self.merge_last_count_ = 0

	/* We have to allocate one more histogram than the maximum number of block
	   types for the current histogram when the meta-block is too big. */
	max_num_types = brotli_min_size_t(max_num_blocks, self.max_block_types_+1)

	brotli_ensure_capacity_uint8_t(&split.types, &split.types_alloc_size, max_num_blocks)
	brotli_ensure_capacity_uint32_t(&split.lengths, &split.lengths_alloc_size, max_num_blocks)
	split.num_blocks = max_num_blocks
	assert(*histograms == nil)
	*histograms_size = max_num_types * num_contexts
	*histograms = make([]HistogramLiteral, (*histograms_size))
	self.histograms_ = *histograms

	/* Clear only current histogram. */
	ClearHistogramsLiteral(self.histograms_[0:], num_contexts)

	self.last_histogram_ix_[1] = 0
	self.last_histogram_ix_[0] = self.last_histogram_ix_[1]
}

/* Does either of three things:
   (1) emits the current block with a new block type;
   (2) emits the current block with the type of the second last block;
   (3) merges the current block with the last block. */
func ContextBlockSplitterFinishBlock(self *ContextBlockSplitter, is_final bool) {
	var split *BlockSplit = self.split_
	var num_contexts uint = self.num_contexts_
	var last_entropy []float64 = self.last_entropy_[:]
	var histograms []HistogramLiteral = self.histograms_

	if self.block_size_ < self.min_block_size_ {
		self.block_size_ = self.min_block_size_
	}

	if self.num_blocks_ == 0 {
		var i uint

		/* Create first block. */
		split.lengths[0] = uint32(self.block_size_)

		split.types[0] = 0

		for i = 0; i < num_contexts; i++ {
			last_entropy[i] = BitsEntropy(histograms[i].data_[:], self.alphabet_size_)
			last_entropy[num_contexts+i] = last_entropy[i]
		}

		self.num_blocks_++
		split.num_types++
		self.curr_histogram_ix_ += num_contexts
		if self.curr_histogram_ix_ < *self.histograms_size_ {
			ClearHistogramsLiteral(self.histograms_[self.curr_histogram_ix_:], self.num_contexts_)
		}

		self.block_size_ = 0
	} else if self.block_size_ > 0 {
		var entropy [BROTLI_MAX_STATIC_CONTEXTS]float64
		var combined_histo []HistogramLiteral = make([]HistogramLiteral, (2 * num_contexts))
		var combined_entropy [2 * BROTLI_MAX_STATIC_CONTEXTS]float64
		var diff = [2]float64{0.0}
		/* Try merging the set of histograms for the current block type with the
		   respective set of histograms for the last and second last block types.
		   Decide over the split based on the total reduction of entropy across
		   all contexts. */

		var i uint
		for i = 0; i < num_contexts; i++ {
			var curr_histo_ix uint = self.curr_histogram_ix_ + i
			var j uint
			entropy[i] = BitsEntropy(histograms[curr_histo_ix].data_[:], self.alphabet_size_)
			for j = 0; j < 2; j++ {
				var jx uint = j*num_contexts + i
				var last_histogram_ix uint = self.last_histogram_ix_[j] + i
				combined_histo[jx] = histograms[curr_histo_ix]
				HistogramAddHistogramLiteral(&combined_histo[jx], &histograms[last_histogram_ix])
				combined_entropy[jx] = BitsEntropy(combined_histo[jx].data_[0:], self.alphabet_size_)
				diff[j] += combined_entropy[jx] - entropy[i] - last_entropy[jx]
			}
		}

		if split.num_types < self.max_block_types_ && diff[0] > self.split_threshold_ && diff[1] > self.split_threshold_ {
			/* Create new block. */
			split.lengths[self.num_blocks_] = uint32(self.block_size_)

			split.types[self.num_blocks_] = byte(split.num_types)
			self.last_histogram_ix_[1] = self.last_histogram_ix_[0]
			self.last_histogram_ix_[0] = split.num_types * num_contexts
			for i = 0; i < num_contexts; i++ {
				last_entropy[num_contexts+i] = last_entropy[i]
				last_entropy[i] = entropy[i]
			}

			self.num_blocks_++
			split.num_types++
			self.curr_histogram_ix_ += num_contexts
			if self.curr_histogram_ix_ < *self.histograms_size_ {
				ClearHistogramsLiteral(self.histograms_[self.curr_histogram_ix_:], self.num_contexts_)
			}

			self.block_size_ = 0
			self.merge_last_count_ = 0
			self.target_block_size_ = self.min_block_size_
		} else if diff[1] < diff[0]-20.0 {
			split.lengths[self.num_blocks_] = uint32(self.block_size_)
			split.types[self.num_blocks_] = split.types[self.num_blocks_-2]
			/* Combine this block with second last block. */

			var tmp uint = self.last_histogram_ix_[0]
			self.last_histogram_ix_[0] = self.last_histogram_ix_[1]
			self.last_histogram_ix_[1] = tmp
			for i = 0; i < num_contexts; i++ {
				histograms[self.last_histogram_ix_[0]+i] = combined_histo[num_contexts+i]
				last_entropy[num_contexts+i] = last_entropy[i]
				last_entropy[i] = combined_entropy[num_contexts+i]
				HistogramClearLiteral(&histograms[self.curr_histogram_ix_+i])
			}

			self.num_blocks_++
			self.block_size_ = 0
			self.merge_last_count_ = 0
			self.target_block_size_ = self.min_block_size_
		} else {
			/* Combine this block with last block. */
			split.lengths[self.num_blocks_-1] += uint32(self.block_size_)

			for i = 0; i < num_contexts; i++ {
				histograms[self.last_histogram_ix_[0]+i] = combined_histo[i]
				last_entropy[i] = combined_entropy[i]
				if split.num_types == 1 {
					last_entropy[num_contexts+i] = last_entropy[i]
				}

				HistogramClearLiteral(&histograms[self.curr_histogram_ix_+i])
			}

			self.block_size_ = 0
			self.merge_last_count_++
			if self.merge_last_count_ > 1 {
				self.target_block_size_ += self.min_block_size_
			}
		}

		combined_histo = nil
	}

	if is_final {
		*self.histograms_size_ = split.num_types * num_contexts
		split.num_blocks = self.num_blocks_
	}
}

/* Adds the next symbol to the current block type and context. When the
   current block reaches the target size, decides on merging the block. */
func ContextBlockSplitterAddSymbol(self *ContextBlockSplitter, symbol uint, context uint) {
	HistogramAddLiteral(&self.histograms_[self.curr_histogram_ix_+context], symbol)
	self.block_size_++
	if self.block_size_ == self.target_block_size_ {
		ContextBlockSplitterFinishBlock(self, false) /* is_final = */
	}
}

func MapStaticContexts(num_contexts uint, static_context_map []uint32, mb *MetaBlockSplit) {
	var i uint
	assert(mb.literal_context_map == nil)
	mb.literal_context_map_size = mb.literal_split.num_types << BROTLI_LITERAL_CONTEXT_BITS
	mb.literal_context_map = make([]uint32, (mb.literal_context_map_size))

	for i = 0; i < mb.literal_split.num_types; i++ {
		var offset uint32 = uint32(i * num_contexts)
		var j uint
		for j = 0; j < 1<<BROTLI_LITERAL_CONTEXT_BITS; j++ {
			mb.literal_context_map[(i<<BROTLI_LITERAL_CONTEXT_BITS)+j] = offset + static_context_map[j]
		}
	}
}

func BrotliBuildMetaBlockGreedyInternal(ringbuffer []byte, pos uint, mask uint, prev_byte byte, prev_byte2 byte, literal_context_lut ContextLut, num_contexts uint, static_context_map []uint32, commands []Command, n_commands uint, mb *MetaBlockSplit) {
	var lit_blocks struct {
		plain BlockSplitterLiteral
		ctx   ContextBlockSplitter
	}
	var cmd_blocks BlockSplitterCommand
	var dist_blocks BlockSplitterDistance
	var num_literals uint = 0
	var i uint
	for i = 0; i < n_commands; i++ {
		num_literals += uint(commands[i].insert_len_)
	}

	if num_contexts == 1 {
		InitBlockSplitterLiteral(&lit_blocks.plain, 256, 512, 400.0, num_literals, &mb.literal_split, &mb.literal_histograms, &mb.literal_histograms_size)
	} else {
		InitContextBlockSplitter(&lit_blocks.ctx, 256, num_contexts, 512, 400.0, num_literals, &mb.literal_split, &mb.literal_histograms, &mb.literal_histograms_size)
	}

	InitBlockSplitterCommand(&cmd_blocks, BROTLI_NUM_COMMAND_SYMBOLS, 1024, 500.0, n_commands, &mb.command_split, &mb.command_histograms, &mb.command_histograms_size)
	InitBlockSplitterDistance(&dist_blocks, 64, 512, 100.0, n_commands, &mb.distance_split, &mb.distance_histograms, &mb.distance_histograms_size)

	for i = 0; i < n_commands; i++ {
		var cmd Command = commands[i]
		var j uint
		BlockSplitterAddSymbolCommand(&cmd_blocks, uint(cmd.cmd_prefix_))
		for j = uint(cmd.insert_len_); j != 0; j-- {
			var literal byte = ringbuffer[pos&mask]
			if num_contexts == 1 {
				BlockSplitterAddSymbolLiteral(&lit_blocks.plain, uint(literal))
			} else {
				var context uint = uint(BROTLI_CONTEXT(prev_byte, prev_byte2, literal_context_lut))
				ContextBlockSplitterAddSymbol(&lit_blocks.ctx, uint(literal), uint(static_context_map[context]))
			}

			prev_byte2 = prev_byte
			prev_byte = literal
			pos++
		}

		pos += uint(CommandCopyLen(&cmd))
		if CommandCopyLen(&cmd) != 0 {
			prev_byte2 = ringbuffer[(pos-2)&mask]
			prev_byte = ringbuffer[(pos-1)&mask]
			if cmd.cmd_prefix_ >= 128 {
				BlockSplitterAddSymbolDistance(&dist_blocks, uint(cmd.dist_prefix_)&0x3FF)
			}
		}
	}

	if num_contexts == 1 {
		BlockSplitterFinishBlockLiteral(&lit_blocks.plain, true) /* is_final = */
	} else {
		ContextBlockSplitterFinishBlock(&lit_blocks.ctx, true) /* is_final = */
	}

	BlockSplitterFinishBlockCommand(&cmd_blocks, true)   /* is_final = */
	BlockSplitterFinishBlockDistance(&dist_blocks, true) /* is_final = */

	if num_contexts > 1 {
		MapStaticContexts(num_contexts, static_context_map, mb)
	}
}

func BrotliBuildMetaBlockGreedy(ringbuffer []byte, pos uint, mask uint, prev_byte byte, prev_byte2 byte, literal_context_lut ContextLut, num_contexts uint, static_context_map []uint32, commands []Command, n_commands uint, mb *MetaBlockSplit) {
	if num_contexts == 1 {
		BrotliBuildMetaBlockGreedyInternal(ringbuffer, pos, mask, prev_byte, prev_byte2, literal_context_lut, 1, nil, commands, n_commands, mb)
	} else {
		BrotliBuildMetaBlockGreedyInternal(ringbuffer, pos, mask, prev_byte, prev_byte2, literal_context_lut, num_contexts, static_context_map, commands, n_commands, mb)
	}
}

func BrotliOptimizeHistograms(num_distance_codes uint32, mb *MetaBlockSplit) {
	var good_for_rle [BROTLI_NUM_COMMAND_SYMBOLS]byte
	var i uint
	for i = 0; i < mb.literal_histograms_size; i++ {
		BrotliOptimizeHuffmanCountsForRle(256, mb.literal_histograms[i].data_[:], good_for_rle[:])
	}

	for i = 0; i < mb.command_histograms_size; i++ {
		BrotliOptimizeHuffmanCountsForRle(BROTLI_NUM_COMMAND_SYMBOLS, mb.command_histograms[i].data_[:], good_for_rle[:])
	}

	for i = 0; i < mb.distance_histograms_size; i++ {
		BrotliOptimizeHuffmanCountsForRle(uint(num_distance_codes), mb.distance_histograms[i].data_[:], good_for_rle[:])
	}
}