package embedbin

// This file is the seam between the reader (this package, part of the v2
// runtime module) and the encoder/verify code that moved to the tools module
// (github.com/ringsaturn/tzf/v2/internal/embedenc), which
// may import this package under Go's path-based internal rule because both
// share the github.com/ringsaturn/tzf/v2 path prefix. Everything exported
// here is still internal to the v2 module — none of it is public API.

// Format vocabulary (spec rev 1 §3–§6).
const (
	HeaderSize      = headerSize
	SectionEntryLen = sectionEntryLen
	FooterSize      = footerSize
	ProfileOffset   = profileOffset
	ProfileE        = profileE
	ProfileM        = profileM
	FlagGrid        = flagGrid
	FlagNoShortcut  = flagNoShortcut
	DefaultChunk    = defaultChunk
	CoordScale      = coordScale

	SectionNames       = sectionNames
	SectionTZDir       = sectionTZDir
	SectionPolyDir     = sectionPolyDir
	SectionRingDir     = sectionRingDir
	SectionRingOps     = sectionRingOps
	SectionGroupDir    = sectionGroupDir
	SectionChunkDir    = sectionChunkDir
	SectionGrid        = sectionGrid
	SectionPoints      = sectionPoints
	SectionFuzzy       = sectionFuzzy
	SectionFlatPoints  = sectionFlatPoints
	SectionFlatRingDir = sectionFlatRingDir
	SectionYStripes    = sectionYStripes
	SectionSlots       = sectionSlots

	TZRecordLen       = tzRecordLen
	PolyRecordLen     = polyRecordLen
	RingRecordLen     = ringRecordLen
	GroupRecordLen    = groupRecordLen
	ChunkRecordLen    = chunkRecordLen
	FlatRingRecordLen = flatRingRecordLen

	FuzzyHeaderLen = fuzzyHeaderLen
	FuzzyMulti     = fuzzyMulti
	FuzzyMaxNames  = fuzzyMaxNames
)

// LockDecode takes the reader's decode lock and invalidates the chunk cache,
// giving white-box callers exclusive use of the shared decode workspace.
func (r *Reader) LockDecode() {
	r.mu.Lock()
	r.work.cacheValid = false
}

// UnlockDecode releases the decode lock taken by LockDecode.
func (r *Reader) UnlockDecode() {
	r.mu.Unlock()
}

// Profile returns the file's profile byte (ProfileE or ProfileM).
func (r *Reader) Profile() byte { return r.profile }

// TZCount returns the file's timezone count.
func (r *Reader) TZCount() uint32 { return r.tzCount }

// PolyCount returns the file's polygon record count.
func (r *Reader) PolyCount() uint32 { return r.polyCount }

// RingCount returns the file's ring record count.
func (r *Reader) RingCount() uint32 { return r.ringCount }

// GroupCount returns the file's shared-edge group count (E profile).
func (r *Reader) GroupCount() uint32 { return r.groupCount }

// Format version, for tests that build headers by hand.
const (
	FormatMajor = formatMajor
	FormatMinor = formatMinor
)
