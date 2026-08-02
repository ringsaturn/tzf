import unittest

from scripts import bench2summary


class Bench2SummaryTest(unittest.TestCase):
    def test_parse_tzb_benchmarks(self):
        text = """
BenchmarkTZBFinder_GetTimezoneName_Random_WorldCities-16  100  6000 ns/op  4800 ns/p50  12000 ns/p90  24000 ns/p99  0 B/op  0 allocs/op
BenchmarkNewFinderFromTZBReaderAt-16  10  2000000 ns/op  67000 B/op  1394 allocs/op
"""
        rows = bench2summary.parse_bench(text)
        self.assertEqual(2, len(rows))
        self.assertEqual("BenchmarkTZBFinder_GetTimezoneName_Random_WorldCities", rows[0]["name"])
        self.assertEqual(4800, rows[0]["ns_p50"])
        self.assertEqual(1394, rows[1]["allocs_op"])

    def test_tzb_lookup_metadata(self):
        self.assertEqual(
            (
                "TZBFinder",
                "topology-simplified .tzb",
                "TZBFinder",
                "random world cities · GetTimezoneName",
            ),
            bench2summary.bench_meta(
                "BenchmarkTZBFinder_GetTimezoneName_Random_WorldCities"
            ),
        )

    def test_tzb_reader_at_metadata(self):
        self.assertEqual(
            (
                "TZBFinder ReaderAt",
                "topology-simplified .tzb",
                "TZBFinderReaderAt",
                "random world cities · GetTimezoneName",
            ),
            bench2summary.bench_meta(
                "BenchmarkTZBFinderReaderAt_GetTimezoneName_Random_WorldCities"
            ),
        )

    def test_tzb_construction_metadata(self):
        self.assertEqual(
            (
                "TZBFinder ReaderAt",
                "topology-simplified .tzb",
                "TZBFinderReaderAt",
                "construction",
            ),
            bench2summary.bench_meta("BenchmarkNewFinderFromTZBReaderAt"),
        )


if __name__ == "__main__":
    unittest.main()
