import os

from setuptools import setup

version = os.popen("git describe --tags --always").read().split("-")[0]

setup(
    name="tzfpy",
    version=version,
    description="The uWSGI server",
    author="ringsaturn",
    author_email="ringsaturn.me@gmail.com",
    license="MIT",
    packages=[""],
    package_dir={"": "."},
    package_data={"": ["tzfpy/tzf.so"]},
)
