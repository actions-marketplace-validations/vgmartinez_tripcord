"""Run the tripcord release binary bundled in this wheel.

The wheel holds the binary from the GitHub release archive for this
platform, unchanged. This module only finds it and runs it with the
same arguments; it does no network access.
"""

import os
import subprocess
import sys


def binary_path():
    """Return the path of the bundled tripcord binary."""
    name = "tripcord.exe" if sys.platform == "win32" else "tripcord"
    return os.path.join(os.path.dirname(os.path.abspath(__file__)), "bin", name)


def main():
    path = binary_path()
    if not os.path.isfile(path):
        sys.stderr.write(
            "tripcord: no bundled binary at %s. This wheel does not match "
            "this platform; reinstall tripcord with pip or uv.\n" % path
        )
        return 1
    args = [path] + sys.argv[1:]
    if sys.platform == "win32":
        try:
            return subprocess.call(args)
        except KeyboardInterrupt:
            return 130
    if not os.access(path, os.X_OK):
        os.chmod(path, 0o755)
    os.execv(path, args)
    return 1
