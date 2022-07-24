import os
import sysconfig

from setuptools import setup
from setuptools.command.build_py import build_py as _build_py
from setuptools.dist import Distribution
from wheel.bdist_wheel import bdist_wheel as _bdist_wheel

version = (
    os.popen("git describe --tags --always")
    .read()
    .replace("-", "")
    .replace("alpha", "a")
    .replace("beta", "b")
)


class bdist_wheel(_bdist_wheel):
    def finalize_options(self):
        _bdist_wheel.finalize_options(self)
        self.root_is_pure = False


class BinaryDistribution(Distribution):
    """Distribution which always forces a binary package with platform name"""

    def has_ext_modules(foo):
        return True


def get_ext_paths(root_dir, exclude_files):
    """get filepaths for compilation"""
    paths = []

    for root, dirs, files in os.walk(root_dir):
        for filename in files:
            if os.path.splitext(filename)[1] != ".py":
                continue

            file_path = os.path.join(root, filename)
            if file_path in exclude_files:
                continue

            paths.append(file_path)
    return paths


# noinspection PyPep8Naming
class build_py(_build_py):
    def find_package_modules(self, package, package_dir):
        ext_suffix = sysconfig.get_config_var("EXT_SUFFIX")
        modules = super().find_package_modules(package, package_dir)
        filtered_modules = []
        for (pkg, mod, filepath) in modules:
            if os.path.exists(filepath.replace(".py", ext_suffix)):
                continue
            filtered_modules.append(
                (
                    pkg,
                    mod,
                    filepath,
                )
            )
        return filtered_modules


setup(
    name="tzfpy",
    version=version,
    description="tzf's Python binding",
    author="ringsaturn",
    author_email="ringsaturn.me@gmail.com",
    license="MIT",
    packages=[""],
    package_dir={"": "."},
    package_data={"": ["tzfpy/tzf.so"]},
    cmdclass={"build_py": build_py, "bdist_wheel": bdist_wheel},
    distclass=BinaryDistribution,
)
