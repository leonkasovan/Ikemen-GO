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

function f_checkFile(file, msg, dirs, list_files)
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
		print(string.format('%s = %s(%s) [%s]', msg, file, found_in, status))
	else
		print(string.format('%s = %s [%s]', msg, file, status))
	end

	if status == "FAIL" and list_files then
		local start_index, end_index
		for k, v in ipairs(list_files) do
			start_index, end_index = string.find(v:lower(), file:lower(), 1, true)
			if start_index and end_index then
				return string.sub(v, start_index, end_index)
			end
		end
	end
	return nil
end

function f_listFiles(dirs)
	files = {}
	for i, dir in ipairs(dirs) do
		for k, v in ipairs(getDirectoryFiles(dir)) do
			table.insert(files, v)
		end
	end
	return files
end

function f_extractDir(path)
	return path:match('^(.-)[^/\\]+$')
end

-------------------------------------------------------------------
-- CHECK config.ini
-------------------------------------------------------------------
local fonts_selection = {}
local storyboards_selection = {}
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

f_checkFile(config_file, "Ikemen Config")
for index, value in ipairs(config.CommonAir) do
	f_checkFile(value, "[config.ini] CommonAir["..tostring(index).."]")
end
for index, value in ipairs(config.CommonCmd) do
	f_checkFile(value, "[config.ini] CommonCmd["..tostring(index).."]")
end
for index, value in ipairs(config.CommonConst) do
	f_checkFile(value, "[config.ini] CommonConst["..tostring(index).."]")
end
for index, value in ipairs(config.CommonStates) do
	f_checkFile(value, "[config.ini] CommonStates["..tostring(index).."]")
end

f_checkFile(config.DebugFont, "[config.ini] DebugFont")
if string.find(config.DebugFont, '.def') then
	table.insert(fonts_selection, config.DebugFont)
end
f_checkFile(config.Motif, "[config.ini] Motif")
local motifDir = f_extractDir(config.Motif)
print(string.format('[config.ini] Motif Directory: %s', motifDir))
f_checkFile(config.StartStage, "[config.ini] StartStage")
f_checkFile(config.System, "[config.ini] System")

for index, value in ipairs(config.WindowIcon) do
	f_checkFile(value, "[config.ini] WindowIcon["..tostring(index).."]")
end

-------------------------------------------------------------------
-- CHECK config.Motif: system.def
-------------------------------------------------------------------
content = f_fileRead(config.Motif)
if content == nil then
	print("[ERROR] Can not read "..config.Motif)
	return
end

local group
local motif = {}
local file, err = io.open(config.Motif, "w")
local modified_line = ""
local list_files = f_listFiles({"font","data","sound",motifDir})
for src_line in content:gmatch('([^\n]*)\n?') do
	line = src_line:gsub('%s*;.*$', '')
	if line:match('^[^%g]*%s*%[.-%s*%]%s*$') then --matched [] group
		line = line:match('%[(.-)%s*%]%s*$') --match text between []
		line = line:gsub('[%. ]', '_') --change . and space to _
		group = tostring(line:lower())
	else --matched non [] line
		local param, value = line:match('^%s*([^=]-)%s*=%s*(.-)%s*$')
		if param ~= nil then
			param = param:gsub('[%. ]', '_') --change param . and space to _
			if group ~= 'glyphs' then
				param = param:lower() --lowercase param
			end
			if value ~= nil then --let's check if it's even a valid param
				if value == '' then --text should remain empty
					value = nil
				end
			end
		end
		if param ~= nil and value ~= nil then --param = value pattern matched
			value = value:gsub('"', '') --remove brackets from value
			value = value:gsub('^(%.[0-9])', '0%1') --add 0 before dot if missing at the beginning of matched string
			value = value:gsub('([^0-9])(%.[0-9])', '%10%2') --add 0 before dot if missing anywhere else
			value = value:gsub(',%s*$', '') --remove dummy ','
			if group == 'files' then
				if param:match('^font[0-9]+') then --font declaration param matched
					motif[param] = searchFile(value, {"font/", motifDir})
					if string.find(motif[param], '.def') then
						table.insert(fonts_selection, motif[param])
					end
				else
					motif[param] = searchFile(value, {"font/","data/","sound/",motifDir})
				end
				local filepath = f_checkFile(motif[param], "[system.def]["..group.."] "..param, {"font/","data/","sound/",motifDir}, list_files)
				if filepath ~= nil then
					modified_line = param .. " = " ..filepath
				end
			elseif group == 'music' then
				if param:match('%_bgm$') then
					param = param:gsub('_','.')
					local filepath = f_checkFile(value, "[system.def]["..group.."] "..param, {"font/","data/","sound/",motifDir}, list_files)
					if filepath ~= nil then
						modified_line = param.. " = " ..filepath
					end
				end
			elseif group == 'titlebgdef' then
				if param == "spr" then
					local filepath = f_checkFile(value, "[system.def]["..group.."] "..param, {"font/","data/","sound/",motifDir}, list_files)
					if filepath ~= nil then
						modified_line = param.. " = " ..filepath
					end
				end
			elseif group == 'selectbgdef' then
				if param == "spr" then
					local filepath = f_checkFile(value, "[system.def]["..group.."] "..param, {"font/","data/","sound/",motifDir}, list_files)
					if filepath ~= nil then
						modified_line = param.. " = " ..filepath
					end
				end
			elseif group == 'continue_screen' then
				if param == "bgm" then
					local filepath = f_checkFile(value, "[system.def]["..group.."] "..param, {"font/","data/","sound/",motifDir}, list_files)
					if filepath ~= nil then
						modified_line = param.. " = " ..filepath
					end
				end
			end

			if param:match("^.-storyboard") then
				local filepath = f_checkFile(value, "[system.def]["..group.."] "..param, {"font/","data/","sound/",motifDir}, list_files)
				if filepath ~= nil then
					modified_line = param.. " = " ..filepath
					table.insert(storyboards_selection, searchFile(filepath, {"font/","data/","sound/",motifDir}))
				else
					table.insert(storyboards_selection, searchFile(value, {"font/","data/","sound/",motifDir}))
				end
			end
		end
	end
	if modified_line == "" then
		if string.find(src_line, '\\') and not string.find(src_line, 'text') then
			src_line = src_line:gsub('\\','/')
			print("[system.def]["..group.."] [FIXED] "..src_line)
		end
		file:write(src_line .. "\n")
	else
		if string.find(modified_line, '\\') then
			modified_line = modified_line:gsub('\\','/')
		end
		print("[system.def]["..group.."] "..modified_line.." [FIXED]")
		file:write(modified_line .. "\n")
	end
	modified_line = ""
end
file:close()

-------------------------------------------------------------------
-- CHECK motif.fight: fight.def
-------------------------------------------------------------------
content = f_fileRead(motif.fight)
if content == nil then
	print("[ERROR] Can not read "..motif.fight)
	return
end

local group
local file, err = io.open(motif.fight, "w")
local modified_line = ""
local fight_dir = f_extractDir(motif.fight)
local list_files = f_listFiles({fight_dir, "font", "data", "sound", motifDir})
for src_line in content:gmatch('([^\n]*)\n?') do
	line = src_line:gsub('%s*;.*$', '')
	if line:match('^[^%g]*%s*%[.-%s*%]%s*$') then --matched [] group
		line = line:match('%[(.-)%s*%]%s*$') --match text between []
		line = line:gsub('[%. ]', '_') --change . and space to _
		group = tostring(line:lower())
	else --matched non [] line
		local param, value = line:match('^%s*([^=]-)%s*=%s*(.-)%s*$')
		if param ~= nil then
			param = param:gsub('[%. ]', '_') --change param . and space to _
			if group ~= 'glyphs' then
				param = param:lower() --lowercase param
			end
			if value ~= nil then --let's check if it's even a valid param
				if value == '' then --text should remain empty
					value = nil
				end
			end
		end
		if param ~= nil and value ~= nil then --param = value pattern matched
			value = value:gsub('"', '') --remove brackets from value
			value = value:gsub('^(%.[0-9])', '0%1') --add 0 before dot if missing at the beginning of matched string
			value = value:gsub('([^0-9])(%.[0-9])', '%10%2') --add 0 before dot if missing anywhere else
			value = value:gsub(',%s*$', '') --remove dummy ','
			if group == 'files' then
				if param:match('^font[0-9]+') then --font declaration param matched
					motif[param] = searchFile(value, {fight_dir, "font/", motifDir})
					if string.find(motif[param], '.def') then
						table.insert(fonts_selection, motif[param])
					end
				else
					motif[param] = searchFile(value, {fight_dir, motifDir, "data/"})
				end
				local filepath = f_checkFile(motif[param], "[fight.def] "..param, nil, list_files)
				if filepath ~= nil then
					modified_line = param:gsub('_','.') .. " = " ..filepath
				end
			end
		end
	end
	if modified_line == "" then
		if string.find(src_line, '\\') then
			src_line = src_line:gsub('\\','/')
			print("[fight.def] [FIXED] "..src_line)
		end
		file:write(src_line .. "\n")
	else
		if string.find(modified_line, '\\') then
			modified_line = modified_line:gsub('\\','/')
		end
		print("[fight.def] "..modified_line.." [FIXED]")
		file:write(modified_line .. "\n")
	end
	modified_line = ""
end
file:close()

-------------------------------------------------------------------
-- CHECK motif.select: select.def
-------------------------------------------------------------------
content = f_fileRead(motif.select)
if content == nil then
	print("[ERROR] Can not read "..motif.select)
	return
end

local lanChars = false
local lanStages = false
local lanOptions = false
local lanStory = false
for src_line in content:gmatch('[^\r\n]+') do
	local lineCase = src_line:lower()
	if lineCase:match('^%s*%[%s*' .. config.Language .. '.characters' .. '%s*%]') then
		lanChars = true
	elseif lineCase:match('^%s*%[%s*' .. config.Language .. '.extrastages' .. '%s*%]') then
		lanStages = true
	elseif lineCase:match('^%s*%[%s*' .. config.Language .. '.options' .. '%s*%]') then
		lanOptions = true
	elseif lineCase:match('^%s*%[%s*' .. config.Language .. '.storymode' .. '%s*%]') then
		lanStory = true
	end
end

local group
local chars_selection = {}
local stages_selection= {}
local file, err = io.open(motif.select, "w")
local modified_line = ""
local list_files = f_listFiles({"chars","stages"})
for src_line in content:gmatch('[^\r\n]+') do
	--~ line = src_line:gsub('([^\r\n;]*)%s*;[^\r\n]*', '%1'):gsub('\n%s*\n', '\n')
	line = src_line:gsub('%s*;.*$', '')
	local lineCase = line:lower()
	if lineCase == "" then
		-- do nothing
	elseif lineCase:match('^%s*%[%s*characters%s*%]') then
		print("[select.def]"..line)
		row = 0
		section = 1
	elseif lineCase:match('^%s*%[%s*' .. config.Language .. '.characters' .. '%s*%]') then
		print("[select.def]"..line)
		if lanChars then
			row = 0
			section = 1
		else
			section = -1
		end
	elseif lineCase:match('^%s*%[%s*extrastages%s*%]') then
		print("[select.def]"..line)
		row = 0
		section = 2
	elseif lineCase:match('^%s*%[%s*' .. config.Language .. '.extrastages' .. '%s*%]') then
		print("[select.def]"..line)
		if lanStages then
			row = 0
			section = 2
		else
			section = -1
		end
	elseif lineCase:match('^%s*%[%s*options%s*%]') then
		print("[select.def]"..line)
		row = 0
		section = 3
	elseif lineCase:match('^%s*%[%s*' .. config.Language .. '.options' .. '%s*%]') then
		print("[select.def]"..line)
		if lanOptions then
			row = 0
			section = 3
		else
			section = -1
		end
	elseif lineCase:match('^%s*%[%s*storymode%s*%]') then
		print("[select.def]"..line)
		row = 0
		section = 4
	elseif lineCase:match('^%s*%[%s*' .. config.Language .. '.storymode' .. '%s*%]') then
		print("[select.def]"..line)
		if lanStory then
			row = 0
			section = 4
		else
			section = -1
		end
	elseif lineCase:match('^%s*%[%w+%]$') then
		print("[select.def]"..line)
		section = -1
	elseif section == 1 then --[Characters]
		if lineCase:match('^%s*slot%s*=%s*{%s*$') then --start of the 'multiple chars in one slot' assignment
		elseif slot and lineCase:match('^%s*}%s*$') then --end of 'multiple chars in one slot' assignment
		else
			if line:lower() ~= "randomselect" and line:lower() ~= "blank" and line ~= "}" and line:lower() ~= "empty" and line:lower() ~= "" then
				local char_def
				local c = f_strsplit(',', line)
				local stripped_ch = c[1]:match("^%s*(.-)%s*$")

				if string.find(stripped_ch, ".def") then
					char_def = stripped_ch
				else
					char_def = stripped_ch.."/"..stripped_ch..".def"
				end
				local filepath = f_checkFile(char_def, "\t"..stripped_ch, {"chars/", motifDir}, list_files)
				if filepath ~= nil then
					table.insert(chars_selection, searchFile(filepath, {"chars/", motifDir}))
					modified_line = src_line:gsub(stripped_ch, filepath:match("^(.-)%/"), 1)
				else
					table.insert(chars_selection, searchFile(char_def, {"chars/", motifDir}))
				end
			end
		end
	elseif section == 2 then --[ExtraStages]
		--store 'unlock' param and get rid of everything that follows it
		local unlock = ''
		local hidden = 0 --TODO: temporary flag, won't be used once stage selection screen is ready
		line = line:gsub(',%s*unlock%s*=%s*(.-)s*$', function(m1)
			unlock = m1
			hidden = 1
			return ''
		end)
		--parse rest of the line
		for i, c in ipairs(f_strsplit(',', line)) do --split using "," delimiter
			c = c:gsub('^%s*(.-)%s*$', '%1')
			if i == 1 then
				local filepath = f_checkFile(c, "\t", {"stages/", motifDir}, list_files)
				if filepath ~= nil then
					table.insert(stages_selection, filepath)
					modified_line = src_line:gsub(c, filepath)
				else
					table.insert(stages_selection, c)
				end
			elseif c:match('^music') then --musicX / musiclife / musicvictory
			else
				local param, value = c:match('^(.-)%s*=%s*(.-)$')
				if param ~= nil and value ~= nil and param ~= '' and value ~= '' then
				end
			end
		end
	elseif section == 3 then --[Options]
		-- skip
	elseif section == 4 then --[StoryMode]
		-- skip
	end

	if modified_line == "" then
		if string.find(src_line, '\\') then
			src_line = src_line:gsub('\\','/')
			print("[select.def] [FIXED] "..src_line)
		end
		file:write(src_line .. "\n")
		--~ print("src_line", src_line)
	else
		if string.find(modified_line, '\\') then
			modified_line = modified_line:gsub('\\','/')
		end
		print("\t"..modified_line.." [FIXED]")
		file:write(modified_line .. "\n")
	end
	modified_line = ""
end
file:close()

-------------------------------------------------------------------
-- CHECK Characters: chars/*/*.def
-------------------------------------------------------------------
for i, ch in ipairs(chars_selection) do
	content = f_fileRead(ch)
	if content == nil then
		print("[ERROR] Can not read chars "..ch)
	else
		print("[select.def] "..ch)

		local group
		local charDir
		local sep

		if string.find(ch, '\\') then
			sep = '\\'
		else
			sep = '/'
		end

		--~ local file, err = io.open(ch..".txt", "w")
		local file, err = io.open(ch, "w")
		local modified_line = ""
		local list_files = f_listFiles({"chars","data","sound"})
		for src_line in content:gmatch('([^\n]*)\n?') do
			line = src_line:gsub('%s*;.*$', '')
			if line:match('^[^%g]*%s*%[.-%s*%]%s*$') then --matched [] group
				line = line:match('%[(.-)%s*%]%s*$') --match text between []
				line = line:gsub('[%. ]', '_') --change . and space to _
				group = tostring(line:lower())
			else --matched non [] line
				local param, value = line:match('^%s*([^=]-)%s*=%s*(.-)%s*$')
				if param ~= nil then
					param = param:gsub('[%. ]', '_') --change param . and space to _
					if value ~= nil then --let's check if it's even a valid param
						if value == '' then --text should remain empty
							value = nil
						end
					end
				end
				if param ~= nil and value ~= nil then --param = value pattern matched
					value = value:gsub('"', '') --remove brackets from value
					value = value:gsub('^(%.[0-9])', '0%1') --add 0 before dot if missing at the beginning of matched string
					value = value:gsub('([^0-9])(%.[0-9])', '%10%2') --add 0 before dot if missing anywhere else
					value = value:gsub(',%s*$', '') --remove dummy ','
					value = value:gsub('%.%./', '')	--remove two sub folder (not needed in chars)

					if group == 'files' or group == 'arcade'then
						local filepath
						charDir = ch:match(".*"..sep)
						filepath = f_checkFile(value, "\t"..param, {charDir, motifDir, "chars"..sep, "data"..sep}, list_files)
						if filepath ~= nil then
							modified_line = param .. " = " ..filepath
						end
					end

				end
			end
			if modified_line == "" then
				if string.find(src_line, '\\') then
					src_line = src_line:gsub('\\','/')
					print("\t[FIXED] "..src_line)
				end
				file:write(src_line .. "\n")
			else
				if string.find(modified_line, '\\') then
					modified_line = modified_line:gsub('\\','/')
				end
				print("\t"..modified_line.." [FIXED]")
				file:write(modified_line .. "\n")
			end
			modified_line = ""
		end
		file:close()
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
		print("[select.def] "..stage)

		local group
		local stageDir
		local sep

		if string.find(stage, '\\') then
			sep = '\\'
		else
			sep = '/'
		end

		--~ local file, err = io.open(stage..".txt", "w")
		local file, err = io.open(stage, "w")
		local modified_line = ""
		local list_files = f_listFiles({"stages","sound","data"})
		for src_line in content:gmatch('([^\n]*)\n?') do
			line = src_line:gsub('%s*;.*$', '')
			if line:match('^[^%g]*%s*%[.-%s*%]%s*$') then --matched [] group
				line = line:match('%[(.-)%s*%]%s*$') --match text between []
				line = line:gsub('[%. ]', '_') --change . and space to _
				group = tostring(line:lower())
			else --matched non [] line
				local param, value = line:match('^%s*([^=]-)%s*=%s*(.-)%s*$')
				if param ~= nil then
					param = param:gsub('[%. ]', '_') --change param . and space to _
					if value ~= nil then --let's check if it's even a valid param
						if value == '' then --text should remain empty
							value = nil
						end
					end
				end
				if param ~= nil and value ~= nil then --param = value pattern matched
					value = value:gsub('"', '') --remove brackets from value
					stageDir = stage:match(".*"..sep)
					if param:lower() == "spr" or param:lower() == "model" or param:lower() == "bgmusic" then
						local filepath = f_checkFile(value, "\t"..param, {stageDir, "stages"..sep, "data"..sep, "sound"..sep}, list_files)
						if filepath ~= nil then
							modified_line = param .. " = " ..filepath
						end
					end
				end
			end
			if modified_line == "" then
				if string.find(src_line, '\\') then
					src_line = src_line:gsub('\\','/')
					print("\t[FIXED] "..src_line)
				end
				file:write(src_line .. "\n")
			else
				if string.find(modified_line, '\\') then
					modified_line = modified_line:gsub('\\','/')
				end
				print("\t"..modified_line.." [FIXED]")
				file:write(modified_line .. "\n")
			end
			modified_line = ""
		end
		file:close()
	end
end

-------------------------------------------------------------------
-- CHECK storyboards: MOTIF_DIR/*.def
-------------------------------------------------------------------
for index, sb in ipairs(storyboards_selection) do
	content = f_fileRead(sb)
	if content == nil then
		print("[ERROR] Can not read storyboard "..sb)
	else
		print("[system.def] Storyboard: "..sb)

		local group
		local sbDir
		local sep

		if string.find(sb, '\\') then
			sep = '\\'
		else
			sep = '/'
		end

		local file, err = io.open(sb, "w")
		local modified_line = ""
		local list_files = f_listFiles({"font","sound","data"})
		for src_line in content:gmatch('([^\n]*)\n?') do
			local line = src_line:gsub('%s*;.*$', '')
			if line:match('^[^%g]*%s*%[.-%s*%]%s*$') then --matched [] group
				line = line:match('%[(.-)%s*%]%s*$') --match text between []
				line = line:gsub('[%. ]', '_') --change . and space to _
				group = tostring(line:lower())
			else --matched non [] line
				local param, value = line:match('^%s*([^=]-)%s*=%s*(.-)%s*$')
				if param ~= nil then
					param = param:gsub('[%. ]', '_') --change param . and space to _
					if value ~= nil then --let's check if it's even a valid param
						if value == '' then --text should remain empty
							value = nil
						end
					end
				end
				if param ~= nil and value ~= nil then --param = value pattern matched
					param = param:lower()
					value = value:gsub('"', '') --remove brackets from value
					sbDir = sb:match(".*"..sep)
					if param == "spr" or param == "snd" or param == "bgm" then
						local filepath = f_checkFile(value, "\t"..param, {sbDir, motifDir, "font"..sep, "sound"..sep}, list_files)
						-- print("\tfilepath", filepath)
						-- print("\tparam", param)
						-- print("\tvalue", value)
						-- print("\tgroup", group)
						-- print("\tsbDir", sbDir)
						-- print("\tmotifDir", motifDir)
						if filepath ~= nil then
							modified_line = param .. " = " ..filepath
						end
					elseif param:find("font[0-9]+") then
						-- print("\tparam", param)
						-- print("\tvalue", value)
						-- print("\tgroup", group)
						-- print("\tsbDir", sbDir)
						-- print("\tmotifDir", motifDir)
						local font_file = searchFile(value, {sbDir, "font/", motifDir})
						-- print("\tfont_file", font_file)
						if string.find(font_file, '%.[Dd][eE][fF]') then
							table.insert(fonts_selection, font_file)
						end
						local filepath = f_checkFile(font_file, "\t"..param, {sbDir, "font/", motifDir}, list_files)
						-- print("\tfilepath", filepath)
						if filepath ~= nil then
							modified_line = param .. " = " ..filepath
						end
					end
				end
			end
			if modified_line == "" then
				if string.find(src_line, '\\') and not string.find(src_line, 'text') then
					src_line = src_line:gsub('\\','/')
					print("\t[FIXED] "..src_line)
				end
				file:write(src_line .. "\n")
			else
				if string.find(modified_line, '\\') then
					modified_line = modified_line:gsub('\\','/')
				end
				print("\t"..modified_line.." [FIXED]")
				file:write(modified_line .. "\n")
			end
			modified_line = ""
		end
		file:close()
	end
end

-------------------------------------------------------------------
-- CHECK Fonts: fonts/*.def
-------------------------------------------------------------------
for index, font in ipairs(fonts_selection) do
	content = f_fileRead(font)
	if content == nil then
		print("[ERROR] Can not read font "..font)
	else
		print("[system.def] Font: "..font)

		local group
		local sep

		if string.find(font, '\\') then
			sep = '\\'
		else
			sep = '/'
		end

		local file, err = io.open(font, "w")
		local fontDir = f_extractDir(font)
		local list_files = f_listFiles({fontDir, "fonts"..sep, "data"..sep})
		for src_line in content:gmatch('([^\n]*)\n?') do
			line = src_line:gsub('%s*;.*$', '')
			if line:match('^[^%g]*%s*%[.-%s*%]%s*$') then --matched [] group
				line = line:match('%[(.-)%s*%]%s*$') --match text between []
				line = line:gsub('[%. ]', '_') --change . and space to _
				group = tostring(line:lower())
			else --matched non [] line
				local param, value = line:match('^%s*([^=]-)%s*=%s*(.-)%s*$')
				if param ~= nil then
					param = param:gsub('[%. ]', '_') --change param . and space to _
					if value ~= nil then --let's check if it's even a valid param
						if value == '' then --text should remain empty
							value = nil
						end
					end
				end
				if param ~= nil and value ~= nil then --param = value pattern matched
					param = param:lower()
					value = value:gsub('"', '') --remove brackets from value
					if param == "file" then
						local filepath = f_checkFile(value, "\t"..param, {fontDir, "fonts"..sep, "data"..sep}, list_files)
						if filepath ~= nil then
							modified_line = param .. " = " ..filepath
						end
					end
				end
			end
			if modified_line == "" then
				if string.find(src_line, '\\') then
					src_line = src_line:gsub('\\','/')
					print("\t[FIXED] "..src_line)
				end
				file:write(src_line .. "\n")
			else
				if string.find(modified_line, '\\') then
					modified_line = modified_line:gsub('\\','/')
				end
				print("\t"..modified_line.." [FIXED]")
				file:write(modified_line .. "\n")
			end
			modified_line = ""
		end
		file:close()
	end
end
