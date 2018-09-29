'''
Copyright 2018 VMware, Inc.
SPDX-License-Identifier: BSD-2-Clause
'''

import argparse
import os
import sys
import tarfile
from elftools.elf.elffile import ELFFile
from elftools.elf.segments import InterpSegment
try:
    import rpm
except ImportError:
    print("WARNING! rpm module not loaded.", file=sys.stderr)
try:
    import apt
except ImportError:
    print("WARNING! apt module not loaded.", file=sys.stderr)


# this is stuff that will never be part of a running container
# /var/lib/docker/ is just in case we are in DIND or there is noise from microdump.lua
IGNORE_SET_FILES = frozenset(["/dev/", "/proc/", "/sys/", "/tmp/", "/var/lib/docker/"])
IGNORE_SET_FOLDERS = frozenset(map(lambda x: x.rstrip("/"), IGNORE_SET_FILES))


class BaseContainer:
    """ This is the base class.
    This class contains all the information we need from the container.
    """


    def __init__(self):

        # a set for all files we need to pack
        self.all_files = set()

        # a set for all folders we need to pack
        self.all_folders = set()

        # a dict for package information, used only by inherited classes
        self.package_info = {}


    def is_folder_needed(self, folder):
        """ Returns if the folder is implicitly contained in others
        Explanation:
        we may have a reference to a folder like /foo in our set,
        if we have later in the set a reference to a file like /foo/bar,
        then the folder /foo is implicit.
        """

        # do we have an implicit folder in the file list?
        for entry in self.all_files:
            if entry.startswith(folder):
                return False

        # do we have an implicit folder in the folder list?
        for entry in self.all_folders:
            if entry.startswith(folder) and entry != folder:
                return False

        # if we couldn't find it...
        return True


    def generate_package_info(self, filename):
        """ Empty function for this base class. """
        pass


class DEBContainer(BaseContainer):
    """ Class for containers that are DEB based. """

    def __init__(self):

        BaseContainer.__init__(self)

        # open a reference to the deb cache
        self.deb_cache = apt.Cache()
        print("WARNING! Licence retrieval is not implemented for DEB containers.", file=sys.stderr)


    def generate_package_info(self, filename):
        """ DEB specific package info generator """

        # skip it for folder
        if os.path.isdir(filename):
            return

        # quite ugly, but it looks we need to traverse the cache
        for deb_cache_entry in self.deb_cache:

            if deb_cache_entry.is_installed and filename in deb_cache_entry.installed_files:

                package = deb_cache_entry.shortname + "-" + deb_cache_entry.versions[0].version + \
                    "-" + deb_cache_entry.architecture() + " (?)"

                # check if package list exists
                if self.package_info.get(package) is None:
                    self.package_info[package] = []

                self.package_info[package].append(filename)
                return

        # we couldn't find anything
        if self.package_info.get("unknown DEB package") is None:
            self.package_info["unknown DEB package"] = []

        self.package_info["unknown DEB package"].append(filename)
        return


class RPMContainer(BaseContainer):
    """ Class for containers that are RPM based. """

    def __init__(self):

        BaseContainer.__init__(self)

        # open a transaction set to the RPM DB
        self.transaction_set = rpm.TransactionSet()


    def generate_package_info(self, filename):
        """ RPM specific package info generator. """

        # skip it for folders
        if os.path.isdir(filename):
            return

        # query the RPM DB for the package this file is coming from
        match_iterator = self.transaction_set.dbMatch("basenames", filename)

        # if you get zero or more than one result, put the file in the "unknown RPM package" list
        if len(match_iterator) != 1:

            # check if the "unknown package" list exists
            if self.package_info.get("unknown RPM package") is None:
                self.package_info["unknown RPM package"] = []

            self.package_info["unknown RPM package"].append(filename)
            return

        # if we have (correctly) only one result
        for header in match_iterator:

            package = header[rpm.RPMTAG_NAME].decode("utf-8") + "-" + \
                header[rpm.RPMTAG_VERSION].decode("utf-8") + "-" + \
                header[rpm.RPMTAG_RELEASE].decode("utf-8") + " (" + \
                header[rpm.RPMTAG_LICENSE].decode("utf-8") + ")"

	# check if the "package" list exists
        if self.package_info.get(package) is None:
            self.package_info[package] = []

        self.package_info[package].append(filename)
        return


def filepaths_from_file(filename):
    """ Return a set of files, following symlinks."""

    file_entries = set()

    if not os.path.islink(filename):

	# just add the file to the set and return
        # don't forget normpath to call to avoid "A/./B" strings
        normalized_filename = os.path.normpath(filename)

        # ignore a normalized_filename that is not supposed to be in a container image
        # don't forget that we might end here after following a symlink
        if any(map(normalized_filename.startswith, IGNORE_SET_FILES)):
            return frozenset()

        # return the single entry as a frozenset
        file_entries.add(normalized_filename)
        return frozenset(file_entries)

    else:

        # as above
        normalized_filename = os.path.normpath(filename)
        if any(map(normalized_filename.startswith, IGNORE_SET_FILES)):
            return frozenset()

        # now add the filename to the set
        file_entries.add(normalized_filename)
        # read the link
        link = os.readlink(normalized_filename)

        # if it is not absolute, create an abs path
        if not os.path.isabs(link):
            link = os.path.normpath(os.path.join(os.path.dirname(normalized_filename), link))

        # we can have a symlink pointing to another symlink
        # /usr/bin/java -> /etc/alternatives/java
        # /etc/alternatives/java -> /usr/lib/jvm/OpenJDK-1.8.0.151/jre/bin/java
        file_entries.update(filepaths_from_file(link))
        return frozenset(file_entries)


def retrieve_loader_from_interp(interp_filename):
    """ Returns the Linux dynamic loader programmatically. """

    with open(interp_filename, "rb") as file:
        for elf_segment in ELFFile(file).iter_segments():
            if isinstance(elf_segment, InterpSegment):
                return elf_segment.get_interp_name()

    # this shouldn't ever fail
    raise SystemExit("ERROR! couldn't find the Linux Loader in file: " + interp_filename)


def main():
    """ Main function of the script. """

    # set up the args parser
    parser = argparse.ArgumentParser()
    parser.add_argument("-i", metavar="input_list", help="list from file", required=True)
    parser.add_argument("-t", metavar="tar_filename", help="tar filename", required=True)
    parser.add_argument("--excl-files", metavar="excluded_file", \
        help="file exclusion list", nargs="+")
    parser.add_argument("--excl-folders", metavar="excluded_folder", \
        help="folder exclusion list", nargs="+")
    parser.add_argument("--interp", metavar="inter_filename", \
        help="retrieve ld-linux.so from ELF", default="/bin/sh", required=False, nargs="?")
    type_group = parser.add_mutually_exclusive_group()
    type_group.add_argument("--rpm", help="rpm based container", action="store_true")
    type_group.add_argument("--deb", help="deb based container", action="store_true")

    args = parser.parse_args()
    namespace = vars(args)

    # add file and folder exclusion lists (reassign the globals)
    if namespace["excl_files"]:
        global IGNORE_SET_FILES
        IGNORE_SET_FILES = frozenset(set(IGNORE_SET_FILES) | set(namespace["excl_files"]))

    if namespace["excl_folders"]:
        global IGNORE_SET_FOLDERS
        IGNORE_SET_FOLDERS = frozenset(set(IGNORE_SET_FOLDERS) | set(namespace["excl_folders"]))

    # create our container instance depending on the specified type
    if namespace["rpm"]:
        container = RPMContainer()
    elif namespace["deb"]:
        container = DEBContainer()
    else:
        container = BaseContainer()

    # read the input file
    with open(namespace["i"], "r") as import_list_file:

        for line in import_list_file.readlines():

            file_string = line.strip("\n")

            # with "isfile" or "isdir" methods, "exists" is implicit
            # also, any "strange" char will get ignored here like "/folder/*"
            if os.path.isfile(file_string):

                # ignore a file that is not supposed to be in a container image
                if any(map(file_string.startswith, IGNORE_SET_FILES)):
                    continue

                # add the file to the set
                container.all_files.update(filepaths_from_file(file_string))

            elif os.path.isdir(file_string):

                # ignore a folder that is not supposed to be in a container image
                if any(map(file_string.startswith, IGNORE_SET_FOLDERS)):
                    continue

                # add the folder to the set
                container.all_folders.add(os.path.normpath(file_string))

            # we may be in the corner case we have a relative string that was in PATH
            elif "/" not in file_string:

                # get PATH env variable
                if "PATH" in os.environ:

                    paths_list = os.environ["PATH"].split(":")
                    for path in paths_list:

                        new_file_string = path + "/" + file_string

                        # check if you found the binary
                        if os.path.isfile(new_file_string):

                            # safety check, you shouldn't land here
                            if any(map(new_file_string.startswith, IGNORE_SET_FILES)):
                                continue

                            # add the file to the set
                            container.all_files.update(filepaths_from_file(new_file_string))
                            break

            # some garbage in file_string?
            else:
                pass

    # end of with-open
    # we have parsed the input file and the container instance has its sets populated
    # we need to add the Linux loader
    interp_filename = namespace["interp"]
    container.all_files.update(filepaths_from_file(retrieve_loader_from_interp(interp_filename)))

    # we want to prune the list of folders we have collected
    needed_folders = frozenset(filter(container.is_folder_needed, container.all_folders))

    # union the two sets
    fs_entries = needed_folders | container.all_files

    # create the tarfile, if a entry is a folder, import recursively
    tar = tarfile.open(namespace["t"], "w")
    for entry in fs_entries:
        container.generate_package_info(entry)
        tar.add(entry)
    tar.close()

    # print the package_info to stdout
    if namespace["rpm"] or namespace["deb"]:
        for key in container.package_info:
            print(key + ":")
            for value in container.package_info[key]:
                print(value)
            print()


# this is a script, not a module
if __name__ == '__main__':
    main()
