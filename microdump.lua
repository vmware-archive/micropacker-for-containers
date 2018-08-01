-- Copyright 2018 VMware, Inc.
-- SPDX-License-Identifier: BSD-2-Clause


-- chisel description
description = "this chisel will print all the file paths included in your capture, including relative commands to PATH"
short_description = "get a list a files to build a microcontainer, use with micropacker.py"
category = "misc"


-- arguments list
args = {}


-- Initialization callback
function on_init()
	-- get some handlers
	exec_h = chisel.request_field("evt.arg.exe")
	file_h = chisel.request_field("evt.arg.filename")
	name_h = chisel.request_field("evt.arg.name")
	path_h = chisel.request_field("evt.arg.path")
	return true
end


-- main event handler
function on_event()
	-- get the fields, then print in an if condition, we want to be fast here
	exec = evt.field(exec_h)
	file = evt.field(file_h) 
	name = evt.field(name_h)
	path = evt.field(path_h)
	if exec ~= nil then
		print(exec)
	end
	if file ~= nil then
		print(file)
	end
	if name ~= nil then
		print(name)
	end	
	if path ~= nil then
		print(path)
	end
	return true
end
