import os
import sysconfig
from distutils.command.install_data import install_data

from setuptools import setup
from setuptools.command.build_py import build_py as _build_py
from setuptools.dist import Distribution


class post_install(install_data):
    def run(self):
        install_data.run(self)
        os.open("make build")


version = (
    os.popen("git describe --tags --always")
    .read()
    .replace("-", "")
    .replace("alpha", "a")
    .replace("beta", "b")
)


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
    cmdclass={"install_data": post_install, "build_py": build_py},
    distclass=BinaryDistribution,
)
