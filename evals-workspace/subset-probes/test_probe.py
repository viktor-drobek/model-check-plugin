import importlib.util
from pathlib import Path
import unittest


spec = importlib.util.spec_from_file_location("probe", Path(__file__).with_name("probe.py"))
probe = importlib.util.module_from_spec(spec)
spec.loader.exec_module(probe)


class PortablePathTest(unittest.TestCase):
    def test_portable_path(self):
        cases = {
            "/checkout/model.pml:12:3": "model.pml:12:3",
            r"/checkout/foo\bar.pml:12:3": r"foo\bar.pml:12:3",
            r"C:\checkout\model.pml:12:3": "model.pml:12:3",
            r"\\server\share\model.pml:12:3": "model.pml:12:3",
            r"\\draft.txt:12:3": r"\\draft.txt:12:3",
            r"é:\reports\draft.txt:12:3": r"é:\reports\draft.txt:12:3",
            "/checkout/model.pml": "model.pml",
            "malformed:line:column": "malformed:line:column",
        }
        for path, want in cases.items():
            with self.subTest(path=path):
                self.assertEqual(probe.portable_path(path), want)

    def test_rooted_windows_path_is_host_aware(self):
        path = r"\Users\alice\work\model.pml:12:3"
        self.assertEqual(probe.portable_path(path, rooted_windows=True), "model.pml:12:3")
        self.assertEqual(probe.portable_path(path, rooted_windows=False), path)


if __name__ == "__main__":
    unittest.main()
