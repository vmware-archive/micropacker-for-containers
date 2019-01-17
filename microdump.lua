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
	args_h = chisel.request_field("evt.args")
	abs_h = chisel.request_field("evt.abspath")
	return true
end


-- main event handler
function on_event()
	-- get the fields if direction is right
	if evt.field(dir_h) == ">" and evt.field(stype_h) ~= nil then
		abs_path = evt.field(abs_h)
		if abs_path ~= nil then
			print(abs_path)
		else
			args = evt.field(args_h)
			if args.filename ~= nil then
				print(args.filename)
			elseif args.name ~= nil then
				print(args.name)
			elseif args.path ~= nil then
				print(args.path)
			end
		end
	end
	return true
end
