package tzf

// Backports support for not updated systems
//
// tzf will try to maintain timezone name backport compatibility until
// new major version release will remove too old names.
var backportstz = map[string]string{
	"Europe/Kyiv": "Europe/Kiev", // [2022b] https://github.com/evansiroky/timezone-boundary-builder/commit/ea87ea5c8bf435d8318a40eb2ab69ea2f7a375aa
}
