package tzf

// Backports support for not updated systems
//
// tzf will try to maintain timezone name backport compatibility until
// new major version release will remove too old names.
var backportstz = map[string]string{
	"Europe/Kyiv":         "Europe/Kiev",      // [2022b] https://github.com/evansiroky/timezone-boundary-builder/releases/tag/2022b commit https://github.com/evansiroky/timezone-boundary-builder/commit/ea87ea5c8bf435d8318a40eb2ab69ea2f7a375aa
	"Europe/Uzhgorod":     "Europe/Kyiv",      // [2022d] https://github.com/evansiroky/timezone-boundary-builder/releases/tag/2022d
	"Europe/Zaporozhye":   "Europe/Kyiv",      // [2022d] https://github.com/evansiroky/timezone-boundary-builder/releases/tag/2022d
	"America/Nipigon":     "America/Toronto",  // [2022f] https://github.com/evansiroky/timezone-boundary-builder/releases/tag/2022f
	"America/Thunder_Bay": "America/Toronto",  // [2022f] https://github.com/evansiroky/timezone-boundary-builder/releases/tag/2022f
	"America/Rainy_River": "America/Winnipeg", // [2022f] https://github.com/evansiroky/timezone-boundary-builder/releases/tag/2022f
}
