-- Copyright 2019 VMware, Inc.
-- SPDX-License-Identifier: BSD-2-Clause


-- chisel description
description = "this chisel will print all the file paths included in your capture, including relative commands to PATH"
short_description = "get a list a files to build a microcontainer, use with ./micropacker"
category = "misc"


-- arguments list
args = {}


-- Initialization callback
function on_init()
	-- get some handlers
	dir_h = chisel.request_field("evt.dir")
	stype_h = chisel.request_field("syscall.type")
	arg_filename_h = chisel.request_field("evt.arg.filename")
	arg_name_h = chisel.request_field("evt.arg.name")
	arg_path_h = chisel.request_field("evt.arg.path")
	arg_exe_h = chisel.request_field("evt.arg.exe")
	absolute_h = chisel.request_field("evt.abspath")
	return true
end


-- main event handler
function on_event()
	-- get the right fields depending on the direction
	if evt.field(dir_h) == "<" then
		-- we might have a > execve with filename=/path/abc.sh
		-- but then we need to capture also /bin/sh in the < execve
		if evt.field(stype_h) == "execve" then
			arg_exe = evt.field(arg_exe_h)
			if arg_exe ~= nil and arg_exe ~= "<NA>" then
				print(arg_exe)
			end
		-- check for name in < open
		elseif evt.field(stype_h) == "open" then
			arg_name = evt.field(arg_name_h)
			if arg_name ~= nil and arg_name ~= "<NA>" then
				print(arg_name)
			end
		end
	end
	if evt.field(dir_h) == ">" and evt.field(stype_h) ~= nil then
		absolute = evt.field(absolute_h)
		if absolute ~= nil then
			print(absolute)
		else
			arg_filename = evt.field(arg_filename_h)
			arg_name = evt.field(arg_name_h)
			arg_path = evt.field(arg_path_h)
			if arg_filename ~= nil and arg_filename ~= "<NA>" then
				print(arg_filename)
			elseif arg_name ~= nil and arg_name ~= "<NA>" then
				print(arg_name)
			elseif arg_path ~= nil and arg_path ~= "<NA>" then
				print(arg_path)
			end
		end
	end
	return true
end
