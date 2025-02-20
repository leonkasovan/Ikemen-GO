--return file content
function f_fileRead(path, mode)
	local file = io.open(path, mode or 'r')
	if file == nil then
		-- panicError("\nFile doesn't exist: " .. path)
		return nil
	end
	local str = file:read("*all")
	file:close()
	return str
end

function checkGroupParam(group, param, check_group_param)
	for k, v in ipairs(check_group_param) do
		if group:lower() == v[1] and #v[2] > 0 and v[2]:find(param:lower()) then
			return true
		elseif group:lower() == v[1] and #v[2] == 0 then
			return true
		end
	end
	return false
end
-------------------------------------------------------------------
-- CHECK config.ini
-------------------------------------------------------------------
-- local filename = "save/config.ini"
-- local check_group_param = {{"common", ",air,cmd,const,states,"}, {"config", ",motif,windowicon,system,gamepadmappings,"}, {"debug",",font,"}}

-- local filename = "chars/cage/cage.def"
-- local check_group_param = {{"files", ""}}

-- local filename = "stages/The_courtyard.def"
-- local check_group_param = {{"music", ",bgmusic,"}, {"bgdef", ",spr,"}}

-- local filename = "data/select.def"
-- local check_group_param = {{"characters", ""}, {"extrastages", ""}}

-- local filename = "data/FIGHT.DEF"
-- local check_group_param = {{"files", ""}}

local filename = "data/System.def"
local check_group_param = {
	{"files", ""}, 
	{"music", ",select.bgm,title.bgm,victory.bgm,"}, 
	{"titlebgdef", ",spr,"}, 
	{"selectbgdef", ",spr,"}, 
	{"continue screen", ",bgm,"},
	{"game over screen", ",storyboard,"},
	{"default ending", ",storyboard,"},
	{"end credits", ",storyboard,"}
}

content = f_fileRead(filename)
if content == nil then
	print("[ERROR] Can not read "..filename)
	return
end

local modified_line = ""
local file, err = io.open(filename..".bak.txt", "w")

-- check it is select.def 
if filename:lower():find("select%.def") then
	for src_line in content:gmatch('([^\n]*)\n?') do
		line = src_line:gsub('%s*;.*$', '')
		if #line == 0 then goto skip end
		if line:match('^[^%g]*%s*%[.-%s*%]%s*$') then --matched [] group
			line = line:match('%[(.-)%s*%]%s*$')   --match text between []
			group = tostring(line:lower())
		else                                       --matched non [] line
			if group ~= nil and checkGroupParam(group, "", check_group_param) then
				modified_line = line:gsub('\\','/')
			end
		end

		::skip::
		if #modified_line > 0 then
			file:writeln(modified_line)
		else
			file:writeln(src_line)
		end
		modified_line = ""
	end
else	-- it's other def file
	for src_line in content:gmatch('([^\n]*)\n?') do
	    line = src_line:gsub('%s*;.*$', '')
		if #line == 0 then goto skip end
		if line:match('^[^%g]*%s*%[.-%s*%]%s*$') then --matched [] group
			line = line:match('%[(.-)%s*%]%s*$')   --match text between []
			group = tostring(line:lower())
		else                                       --matched non [] line
			local param, value = line:match('^%s*([^=]-)%s*=%s*(.-)%s*$')
	        if group ~= nil and param ~= nil and checkGroupParam(group, param, check_group_param) then
				modified_line = param:lower() .. " = " .. value:gsub('\\','/')
			end

			if value ~= nil then
				if value:find("%.") then
					print("DEBUG", group, param, "value", value)
				end
			end
	    end

		::skip::
		if #modified_line > 0 then
			file:writeln(modified_line)
		else
			file:writeln(src_line)
		end
		modified_line = ""
	end
end
file:close()