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
		if group:lower() == v[1] 
		and v[2]:find(param:lower()) then
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
-- local check_group_param = {{"files", ",cmd,cns,sprite,anim,sound,pal1,pal2,pa3,pal4,pal5,pal6,pal7,pal8,pal9,pal10,stcommon,st,st1,st2,st3,st4,st5,st6,st7,st8,st9"}}

local filename = "stages/The_courtyard.def"
local check_group_param = {{"music", ",bgmusic,"}, {"bgdef", ",spr,"}}

content = f_fileRead(filename)
if content == nil then
	f_validation:writeln("[ERROR] Can not read "..filename)
	return
end

local modified_line = ""
local file, err = io.open(filename..".bak.txt", "w")
for src_line in content:gmatch('([^\n]*)\n?') do
    line = src_line:gsub('%s*;.*$', '')
	if #line == 0 then goto skip end
	if line:match('^[^%g]*%s*%[.-%s*%]%s*$') then --matched [] group
		line = line:match('%[(.-)%s*%]%s*$')   --match text between []
		line = line:gsub('[%. ]', '_')         --change . and space to _
		group = tostring(line:lower())
	else                                       --matched non [] line
		local param, value = line:match('^%s*([^=]-)%s*=%s*(.-)%s*$')
        if group ~= nil and param ~= nil and checkGroupParam(group, param, check_group_param) then
			modified_line = param .. " = " .. value:gsub('\\','/')
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
file:close()