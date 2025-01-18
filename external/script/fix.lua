-- Lua script for verifying Ikemen's game assets
-- Dhani.Novan@gmail.com
-- 20.10 Selasa, 01 Oktober 2024

--One-time load of the json routines
json = (loadfile 'external/script/json.lua')()
path_sep = '/'
--;===========================================================
--; COMMON FUNCTIONS
--;===========================================================

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

--check if file exists
function f_fileExists(file)
	if file == '' then
		return false
	end
	local f = io.open(file,'r')
	if f ~= nil then
		io.close(f)
		return true
	end
	f = io.open(file:gsub('\\', path_sep) ,'r')
	if f ~= nil then
		io.close(f)
		return true
	end
	return false
end

--ensure that correct data type is set
function f_dataType(arg)
	arg = arg:gsub('^%s*(.-)%s*$', '%1')
	if tonumber(arg) then
		arg = tonumber(arg)
	elseif arg == 'true' then
		arg = true
	elseif arg == 'false' then
		arg = false
	else
		arg = tostring(arg)
	end
	return arg
end

--split strings
function f_strsplit(delimiter, text)
	local list = {}
	local pos = 1
	if string.find('', delimiter, 1) then
		if string.len(text) == 0 then
			table.insert(list, text)
		else
			for i = 1, string.len(text) do
				table.insert(list, string.sub(text, i, i))
			end
		end
	else
		while true do
			local first, last = string.find(text, delimiter, pos)
			if first then
				table.insert(list, string.sub(text, pos, first - 1))
				pos = last + 1
			else
				table.insert(list, string.sub(text, pos))
				break
			end
		end
	end
	return list
end

function f_checkFile(file, msg, dirs)
	local found_in = ""
	if #file == 0 then 
		status = "n/a"
	else
		if dirs == nil then
			if f_fileExists(file) then status = "OK" else status = "FAIL" end
		else
			if f_fileExists(file) then status = "OK" else status = "FAIL" end
			for index, value in ipairs(dirs) do
				if status == "FAIL" then
					if f_fileExists(value..file) then
						status = "OK"
						found_in = value..file
					end
				end
			end
		end
	end
	if #found_in > 0 then
		print(string.format('%s: %s(%s) [%s]', msg, file, found_in, status))
	else
		print(string.format('%s: %s [%s]', msg, file, status))
	end
	return status == "OK"
end

function f_changeLinuxPath(path)
    return path:gsub("\\", "/")
end

-------------------------------------------------------------------
-- CHECK config.ini
-------------------------------------------------------------------
local fonts_selection = {}
local content
local config_file = "save/config.ini"

local config = {}
config.Language = gameOption('Config.Language')
config.CommonAir = gameOption('Common.Air')
config.CommonCmd = gameOption('Common.Cmd')
config.CommonConst = gameOption('Common.Const')
config.CommonStates = gameOption('Common.States')
config.DebugFont = gameOption('Debug.Font')
config.Motif = gameOption('Config.Motif')
config.StartStage = gameOption('Debug.StartStage')
config.System = gameOption('Config.System')
config.WindowIcon = gameOption('Config.WindowIcon')

if config.Language == nil then
	config.Language = ""
end
table.insert(fonts_selection, config.DebugFont)

-------------------------------------------------------------------
-- CHECK config.Motif: system.def
-------------------------------------------------------------------
print("Checking "..config.Motif)
content = f_fileRead(config.Motif)
if content == nil then
	print("[ERROR] Can not read "..config.Motif)
	return
end
local file = io.open(config.Motif..".fix", 'w')
if file == nil then
	print("[ERROR] Can write file: "..config.Motif..".fix")
	return nil
end
local group
local motif = {}
local new_code = ""
for line in content:gmatch('([^\n]*)\n?') do
	new_code = ""
	line = line:match("^%s*(.-)%s*$")	-- trim whitespace
	local code, comment = line:match("^(.*)(;.*)$")
	if code == nil and comment == nil then code = line end
	if string.len(code) > 0 then
		if code:match('^[^%g]*%s*%[.-%s*%]%s*$') then --matched [] group
			group = code:match('%[(.-)%s*%]%s*$') --match text between []
			group = group:gsub('[%. ]', '_') --change . and space to _
			group = tostring(group:lower())
		else --matched non [] line
			if group == 'files' then
				local param, value = code:match('^%s*([^=]-)%s*=%s*(.-)%s*$')
				param = param:lower()
				if param:match('^font[0-9]+') then --font declaration param matched
					motif[param] = searchFile(value, {"font/", motifDir, "data/"})
					table.insert(fonts_selection, motif[param])
				else
					motif[param] = searchFile(value, {motifDir, "data/"})
				end
-- 				new_code = string.format("%s = %s", param, value:lower())	-- 1st version: dir+filename change to lowercase

				local dir, filename = value:match("^(.*)(/.*)$")	-- 2nd version: only filename change to lowercase
				if filename == nil then
					filename = value
					dir = ""
				end
				new_code = string.format("%s = %s%s", param, dir, filename:lower())
			end
		end
	end
	if #new_code > 0 then
		if comment then
			file:write(new_code.."\t"..comment.."\n")
		else
			file:write(new_code.."\n")
		end
	else
		file:write(line.."\n")
	end
end
file:close()
os.rename(config.Motif, config.Motif..".bak")
os.rename(config.Motif..".fix", config.Motif)

-------------------------------------------------------------------
-- CHECK motif.select: select.def
-------------------------------------------------------------------
content = f_fileRead(motif.select)
if content == nil then
	print("[ERROR] Can not read "..motif.select)
	return
end

local file = io.open(motif.select..".fix", 'w')
if file == nil then
	print("[ERROR] Can write file: "..motif.select..".fix")
	return nil
end
local group
local chars_selection = {}
local stages_selection= {}
local new_code = ""
for line in content:gmatch('([^\n]*)\n?') do
	new_code = ""
	line = line:match("^%s*(.-)%s*$")	-- trim whitespace
	local code, comment = line:match("^(.*)(;.*)$")
	if code == nil and comment == nil then code = line end
	if string.len(code) > 0 then
		if code:match('^[^%g]*%s*%[.-%s*%]%s*$') then --matched [] group
			group = code:match('%[(.-)%s*%]%s*$') --match text between []
			group = group:gsub('[%. ]', '_') --change . and space to _
			group = tostring(group:lower())
		else --matched non [] line
			if string.find(group, 'extrastages') then
				local c = f_strsplit(',', code)
				local dir, filename = f_changeLinuxPath(c[1]):match("^(.*)(/.*)$")	-- 2nd version: only filename change to lowercase
				if filename == nil then
					filename = c[1]
					dir = ""
				end
				new_code = dir..filename:lower()
				table.insert(stages_selection, new_code)
				if c[2] then new_code = new_code..","..c[2] end
				if c[3] then new_code = new_code..","..c[3] end
			elseif string.find(group, 'characters') then
				if code:lower() ~= "randomselect" and code:lower() ~= "blank" and code ~= "}" and code:lower() ~= "empty" then
					local char_found
					local dir, filename
					local c = f_strsplit(',', code)
					local stripped_char = c[1]:match("^%s*(.-)%s*$")	-- trim whitespace in char
					local stripped_stage, stripped_column3, stripped_column4
					if c[2] == nil then
						stripped_stage = ""
					else
						stripped_stage = c[2]:match("^%s*(.-)%s*$") -- trim whitespace in stage
					end

					if c[3] == nil then
						stripped_column3 = ""
					else
						stripped_column3 = c[3]:match("^%s*(.-)%s*$") -- trim whitespace
					end

					if c[4] == nil then
						stripped_column4 = ""
					else
						stripped_column4 = c[4]:match("^%s*(.-)%s*$") -- trim whitespace
					end

					if string.find(stripped_char, ".def") then	-- check if char contains "*.def" file
						dir, filename = f_changeLinuxPath(stripped_char):match("^(.*)(/.*)$")
						if filename == nil then
							filename = stripped_char
							dir = ""
						end
						stripped_char = dir..filename:lower()
						char_found = searchFile(stripped_char, {motifDir, "chars/"})
					else
						char_found = searchFile(stripped_char.."/"..stripped_char..".def", {motifDir, "chars/"})
					end
					table.insert(chars_selection, char_found)

					if string.find(stripped_stage, ".def") then	-- check if stage contains "*.def" file
						dir, filename = f_changeLinuxPath(stripped_stage):match("^(.*)(/.*)$")	-- 2nd version: only filename change to lowercase
						if filename == nil then
							filename = stripped_stage
							dir = ""
						end
					else
						filename = stripped_stage
						dir = ""
					end
					new_code = string.format("%s, %s%s, %s, %s", stripped_char, dir, filename:lower(), stripped_column3, stripped_column4)
				end
			end
		end
	end
	if #new_code > 0 then
		if comment then
			file:write(new_code.."\t"..comment.."\n")
		else
			file:write(new_code.."\n")
		end
	else
		file:write(line.."\n")
	end
end
file:close()
os.rename(motif.select, motif.select..".bak")
os.rename(motif.select..".fix", motif.select)

-------------------------------------------------------------------
-- CHECK Characters: chars/*/*.def
-------------------------------------------------------------------
for i, ch in ipairs(chars_selection) do
	content = f_fileRead(ch)
	if content == nil then
		print("[ERROR] Can not read chars "..ch)
	else
		local group
		local file = io.open(ch..".fix", 'w')
		if file == nil then
			print("[ERROR] Can write file: "..ch..".fix")
			return nil
		end

		local new_code = ""
		for line in content:gmatch('([^\n]*)\n?') do
			new_code = ""
			line = line:match("^%s*(.-)%s*$")	-- trim whitespace
			local code, comment = line:match("^(.*)(;.*)$")
			if code == nil and comment == nil then code = line end
			if string.len(code) > 0 then
				if code:match('^[^%g]*%s*%[.-%s*%]%s*$') then --matched [] group
					group = code:match('%[(.-)%s*%]%s*$') --match text between []
					group = group:gsub('[%. ]', '_') --change . and space to _
					group = tostring(group:lower())
				elseif group then --matched non [] line
					if string.find(group, 'files') or string.find(group, 'arcade') then
						local param, value = code:match('^%s*([^=]-)%s*=%s*(.-)%s*$')
						local dir, filename = f_changeLinuxPath(value):match("^(.*)(/.*)$")	-- 2nd version: only filename change to lowercase
						if filename == nil then
							filename = value
							dir = ""
						end
						new_code = string.format("%s = %s%s", param, dir, filename:lower())
					end
				end
			end
			if #new_code > 0 then
				if comment then
					file:write(new_code.."\t"..comment.."\n")
-- 					print(new_code.."\t"..comment)
				else
					file:write(new_code.."\n")
-- 					print(new_code)
				end
			else
				file:write(line.."\n")
-- 				print(line)
			end
		end
		file:close()
		os.rename(ch, ch..".bak")
		os.rename(ch..".fix", ch)
	end
end

-------------------------------------------------------------------
-- CHECK Stages: stages/*.def
-------------------------------------------------------------------
for index, stage in ipairs(stages_selection) do
	content = f_fileRead(stage)
	if content == nil then
		print("[ERROR] Can not read stage "..stage)
	else
-- 		print("DEBUG", stage)
		local group
		local file = io.open(stage..".fix", 'w')
		if file == nil then
			print("[ERROR] Can write file: "..stage..".fix")
			return nil
		end
		local stageDir
		local new_code = ""
		for line in content:gmatch('([^\n]*)\n?') do
			new_code = ""
			line = line:match("^%s*(.-)%s*$")	-- trim whitespace
			local code, comment = line:match("^(.*)(;.*)$")
			if code == nil and comment == nil then code = line end
			if string.len(code) > 0 then
				if code:match('^[^%g]*%s*%[.-%s*%]%s*$') then --matched [] group
					group = code:match('%[(.-)%s*%]%s*$') --match text between []
					group = group:gsub('[%. ]', '_') --change . and space to _
					group = tostring(group:lower())
				elseif group then --matched non [] line
					if string.find(group, 'music') or string.find(group, 'bgdef') then
-- 						print("DEBUG", code)
						local param, value = code:match('^%s*([^=]-)%s*=%s*(.-)%s*$')
						if value then
							local dir, filename = f_changeLinuxPath(value):match("^(.*)(/.*)$")	-- only filename change to lowercase
							if filename == nil then
								filename = value
								dir = ""
							end
							new_code = string.format("%s = %s%s", param, dir, filename:lower())
						end
					end
				end
			end
			if #new_code > 0 then
				if comment then
					file:write(new_code.."\t"..comment.."\n")
-- 					print(new_code.."\t"..comment)
				else
					file:write(new_code.."\n")
-- 					print(new_code)
				end
			else
				file:write(line.."\n")
-- 				print(line)
			end
		end
		file:close()
		os.rename(stage, stage..".bak")
		os.rename(stage..".fix", stage)
	end
end

-------------------------------------------------------------------
-- CHECK Fonts: fonts/*.def
-------------------------------------------------------------------
-- for index, font in ipairs(fonts_selection) do
-- 	if string.find(font, '.def') then
-- 		content = f_fileRead(font)
-- 		if content == nil then
-- 			print("[ERROR] Can not read chars "..font)
-- 			return
-- 		end
-- 		print("[system.def] "..font)
--
-- 		local group
-- 		local fontDir
-- 		local sep
--
-- 		if string.find(font, '\\') then
-- 			sep = '\\'
-- 		else
-- 			sep = '/'
-- 		end
--
-- 		for line in content:gmatch('([^\n]*)\n?') do
-- 			line = line:gsub('%s*;.*$', '')
-- 			if line:match('^[^%g]*%s*%[.-%s*%]%s*$') then --matched [] group
-- 				line = line:match('%[(.-)%s*%]%s*$') --match text between []
-- 				line = line:gsub('[%. ]', '_') --change . and space to _
-- 				group = tostring(line:lower())
-- 			else --matched non [] line
-- 				local param, value = line:match('^%s*([^=]-)%s*=%s*(.-)%s*$')
-- 				if param ~= nil then
-- 					param = param:gsub('[%. ]', '_') --change param . and space to _
-- 					if value ~= nil then --let's check if it's even a valid param
-- 						if value == '' then --text should remain empty
-- 							value = nil
-- 						end
-- 					end
-- 				end
-- 				if param ~= nil and value ~= nil then --param = value pattern matched
-- 					param = param:lower()
-- 					value = value:gsub('"', '') --remove brackets from value
-- 					fontDir = font:match(".*"..sep)
-- 					if param == "file" then
-- 						f_checkFile(value, "\t"..param, {fontDir, "fonts"..sep, "data"..sep})
-- 					end
-- 				end
-- 			end
-- 		end
-- 	else
-- 		print("non DEF file font: "..font)
-- 	end
-- end

