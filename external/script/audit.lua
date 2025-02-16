-- Lua script for verifying Ikemen's game assets
-- Dhani.Novan@gmail.com
-- 20.10 Selasa, 01 Oktober 2024

path_sep = '/'
local f_validation = io.open("validation" .. os.date("%Y%m%d_%H%M%S") .. ".txt", "w")
if f_validation == nil then
	print("Error: Can not create validation.txt")
	return
end
--;===========================================================
--; COMMON FUNCTIONS
--;===========================================================
-- insert UNIQUE item into a tt table
function f_table_insert(tt, item)
	local found = false
	for k, v in ipairs(tt) do
		if v == item then
			found = true
			break
		end
	end

	if not found then
		table.insert(tt, item)
	end
	return found
end

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

function f_checkFile(file, msg)
	valid_path, rc = findFile(file, { "" })
	if rc == 0 then
		f_validation:writeln(string.format("%s = %s [OK]", msg, valid_path))
	elseif rc == 1 then
		f_validation:writeln(string.format("%s = %s [NOT OK]", msg, valid_path))
	elseif rc == -1 then
		f_validation:writeln(string.format("%s = %s [NOT FOUND]", msg, valid_path))
	elseif rc == -2 then
		f_validation:writeln(string.format("%s = %s [ERROR]", msg, valid_path))
	end
	return nil
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
	f_checkFile(value, "[config.ini] CommonAir[" .. tostring(index) .. "]")
end
for index, value in ipairs(config.CommonCmd) do
	f_checkFile(value, "[config.ini] CommonCmd[" .. tostring(index) .. "]")
end
for index, value in ipairs(config.CommonConst) do
	f_checkFile(value, "[config.ini] CommonConst[" .. tostring(index) .. "]")
end
for index, value in ipairs(config.CommonStates) do
	f_checkFile(value, "[config.ini] CommonStates[" .. tostring(index) .. "]")
end

f_checkFile(config.DebugFont, "[config.ini] DebugFont")
if string.find(config.DebugFont, '%.[Dd][eE][fF]') then
	table.insert(fonts_selection, config.DebugFont)
end
f_checkFile(config.Motif, "[config.ini] Motif")
local motifDir = f_extractDir(config.Motif)
f_validation:writeln(string.format('[config.ini] Motif Directory: %s', motifDir))
f_checkFile(config.StartStage, "[config.ini] StartStage")
f_checkFile(config.System, "[config.ini] System")

for index, value in ipairs(config.WindowIcon) do
	f_checkFile(value, "[config.ini] WindowIcon[" .. tostring(index) .. "]")
end

-------------------------------------------------------------------
-- CHECK config.Motif: system.def
-------------------------------------------------------------------
content = f_fileRead(config.Motif)
if content == nil then
	f_validation:writeln("[ERROR] Can not read " .. config.Motif)
	return
end

local group
local motif = {}
local file, err = io.open(config.Motif, "w")
local modified_line = ""
for src_line in content:gmatch('([^\n]*)\n?') do
	line = src_line:gsub('%s*;.*$', '')
	if line:match('^[^%g]*%s*%[.-%s*%]%s*$') then --matched [] group
		line = line:match('%[(.-)%s*%]%s*$')   --match text between []
		line = line:gsub('[%. ]', '_')         --change . and space to _
		group = tostring(line:lower())
	else                                       --matched non [] line
		local param, value = line:match('^%s*([^=]-)%s*=%s*(.-)%s*$')
		if param ~= nil then
			param = param:gsub('[%. ]', '_') --change param . and space to _
			if group ~= 'glyphs' then
				param = param:lower() --lowercase param
			end
			if value ~= nil then    --let's check if it's even a valid param
				if value == '' then --text should remain empty
					value = nil
				end
			end
		end
		if param ~= nil and value ~= nil then --param = value pattern matched
			local valid_path
			local rc = 99
			value = value:gsub('"', '')             --remove brackets from value
			value = value:gsub('^(%.[0-9])', '0%1') --add 0 before dot if missing at the beginning of matched string
			value = value:gsub('([^0-9])(%.[0-9])', '%10%2') --add 0 before dot if missing anywhere else
			value = value:gsub(',%s*$', '')         --remove dummy ','
			if group == 'files' then
				if param:match('^font[0-9]+') then  --font declaration param matched
					valid_path, rc = findFile(value, { "", "font", motifDir })
					if string.find(valid_path, '%.[Dd][eE][fF]') then
						table.insert(fonts_selection, valid_path)
					end
				elseif param:match("^.-storyboard") then --storyboard param matched
					valid_path, rc = findFile(value, { "", motifDir, "data/" })
					if string.find(valid_path, '%.[Dd][eE][fF]') then
						table.insert(storyboards_selection, valid_path)
					end
				else
					valid_path, rc = findFile(value, { "", "data", "sound", motifDir, "font" })
				end
				motif[param] = valid_path
			elseif group == 'music' then
				if param:match('%_bgm$') then
					param = param:gsub('_', '.')
					valid_path, rc = findFile(value, { "", "data", "sound", motifDir })
				end
			else
				if param == "spr" then
					valid_path, rc = findFile(value, { "", "data", motifDir })
				end
				if param == "bgm" then
					valid_path, rc = findFile(value, { "", "sound", motifDir })
				end
				if param:match("^.-storyboard") then
					valid_path, rc = findFile(value, { "", motifDir, "data/", "font/", "sound/" })
					if string.find(valid_path, '%.[Dd][eE][fF]') then
						table.insert(storyboards_selection, valid_path)
					end
				end
			end

			if rc == 0 then
				f_validation:writeln(string.format("[%s][%s] %s = %s [OK]", config.Motif, group, param, value))
			elseif rc == 1 then
				f_validation:writeln(string.format("[%s][%s] %s = %s(%s) [FIXED]", config.Motif, group, param, value,
					valid_path))
				modified_line = param .. " = " .. valid_path
			elseif rc == -1 then
				f_validation:writeln(string.format("[%s][%s] %s = %s [NOT FOUND]", config.Motif, group, param, value))
			elseif rc == -2 then
				f_validation:writeln(string.format("[%s][%s] %s = %s [ERROR]", config.Motif, group, param, value))
			end
		end
	end
	if modified_line == "" then
		if string.find(src_line, '\\') and not string.find(src_line, 'text') then
			src_line = src_line:gsub('\\', '/')
			f_validation:writeln(string.format("[%s][%s] %s [FIXED]", config.Motif, group, src_line))
		end
		file:write(src_line .. "\n")
	else
		if string.find(modified_line, '\\') then
			modified_line = modified_line:gsub('\\', '/')
		end
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
	f_validation:writeln("[ERROR] Can not read " .. motif.fight)
	return
end

local group
local file, err = io.open(motif.fight, "w")
local modified_line = ""
local fight_dir = f_extractDir(motif.fight)
for src_line in content:gmatch('([^\n]*)\n?') do
	line = src_line:gsub('%s*;.*$', '')
	if line:match('^[^%g]*%s*%[.-%s*%]%s*$') then --matched [] group
		line = line:match('%[(.-)%s*%]%s*$')   --match text between []
		line = line:gsub('[%. ]', '_')         --change . and space to _
		group = tostring(line:lower())
	else                                       --matched non [] line
		local param, value = line:match('^%s*([^=]-)%s*=%s*(.-)%s*$')
		if param ~= nil then
			param = param:gsub('[%. ]', '_') --change param . and space to _
			if group ~= 'glyphs' then
				param = param:lower() --lowercase param
			end
			if value ~= nil then    --let's check if it's even a valid param
				if value == '' then --text should remain empty
					value = nil
				end
			end
		end
		if param ~= nil and value ~= nil then --param = value pattern matched
			local valid_path
			local rc = 99
			local fightDir = f_extractDir(motif.fight)
			value = value:gsub('"', '')             --remove brackets from value
			value = value:gsub('^(%.[0-9])', '0%1') --add 0 before dot if missing at the beginning of matched string
			value = value:gsub('([^0-9])(%.[0-9])', '%10%2') --add 0 before dot if missing anywhere else
			value = value:gsub(',%s*$', '')         --remove dummy ','
			if group == 'files' then
				if param:match('^font[0-9]+') then  --font declaration param matched
					valid_path, rc = findFile(value, { "", fightDir, "font", motifDir })
					if string.find(valid_path, '%.[Dd][eE][fF]') then
						table.insert(fonts_selection, valid_path)
					end
				else
					valid_path, rc = findFile(value, { "", fightDir, "data", "sound", motifDir, "font" })
				end
			end

			if rc == 0 then
				f_validation:writeln(string.format("[%s][%s] %s = %s [OK]", motif.fight, group, param, value))
			elseif rc == 1 then
				f_validation:writeln(string.format("[%s][%s] %s = %s(%s) [FIXED]", motif.fight, group, param, value,
					valid_path))
				modified_line = param .. " = " .. valid_path
			elseif rc == -1 then
				f_validation:writeln(string.format("[%s][%s] %s = %s [NOT FOUND]", motif.fight, group, param, value))
			elseif rc == -2 then
				f_validation:writeln(string.format("[%s][%s] %s = %s [ERROR]", motif.fight, group, param, value))
			end
		end
	end
	if modified_line == "" then
		if string.find(src_line, '\\') then
			src_line = src_line:gsub('\\', '/')
			f_validation:writeln("[fight.def] [FIXED] " .. src_line)
		end
		file:write(src_line .. "\n")
	else
		if string.find(modified_line, '\\') then
			modified_line = modified_line:gsub('\\', '/')
		end
		f_validation:writeln("[fight.def] " .. modified_line .. " [FIXED]")
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
	f_validation:writeln("[ERROR] Can not read " .. motif.select)
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
local stages_selection = {}
local file, err = io.open(motif.select, "w")
local modified_line = ""
for src_line in content:gmatch('[^\r\n]+') do
	line = src_line:gsub('%s*;.*$', '')
	local lineCase = line:lower()
	if lineCase == "" then
		-- do nothing
	elseif lineCase:match('^%s*%[%s*characters%s*%]') then
		f_validation:writeln("[select.def]" .. line)
		row = 0
		section = 1
	elseif lineCase:match('^%s*%[%s*' .. config.Language .. '.characters' .. '%s*%]') then
		f_validation:writeln("[select.def]" .. line)
		if lanChars then
			row = 0
			section = 1
		else
			section = -1
		end
	elseif lineCase:match('^%s*%[%s*extrastages%s*%]') then
		f_validation:writeln("[select.def]" .. line)
		row = 0
		section = 2
	elseif lineCase:match('^%s*%[%s*' .. config.Language .. '.extrastages' .. '%s*%]') then
		f_validation:writeln("[select.def]" .. line)
		if lanStages then
			row = 0
			section = 2
		else
			section = -1
		end
	elseif lineCase:match('^%s*%[%s*options%s*%]') then
		f_validation:writeln("[select.def]" .. line)
		row = 0
		section = 3
	elseif lineCase:match('^%s*%[%s*' .. config.Language .. '.options' .. '%s*%]') then
		f_validation:writeln("[select.def]" .. line)
		if lanOptions then
			row = 0
			section = 3
		else
			section = -1
		end
	elseif lineCase:match('^%s*%[%s*storymode%s*%]') then
		row = 0
		section = 4
	elseif lineCase:match('^%s*%[%s*' .. config.Language .. '.storymode' .. '%s*%]') then
		if lanStory then
			row = 0
			section = 4
		else
			section = -1
		end
	elseif lineCase:match('^%s*%[%w+%]$') then
		section = -1
	elseif section == 1 then                       --[Characters]
		if lineCase:match('^%s*slot%s*=%s*{%s*$') then --start of the 'multiple chars in one slot' assignment
		elseif slot and lineCase:match('^%s*}%s*$') then --end of 'multiple chars in one slot' assignment
		else
			if line:lower() ~= "randomselect" and line:lower() ~= "blank" and line ~= "}" and line:lower() ~= "empty" and line:lower() ~= "" then
				local char_def
				local c = f_strsplit(',', line)
				local stripped_ch = c[1]:match("^%s*(.-)%s*$")
				local stripped_stage = ""
				local rc = 99

				if c[2] ~= nil then -- 2nd column is stage definition
					stripped_stage = c[2]:match("^%s*(.-)%s*$")
					if stripped_stage ~= "" and stripped_stage:lower() ~= "random" then
						valid_path, rc = findFile(stripped_stage, { "", "stages" })
						if rc == 0 then -- if found or fixed then add into stages_selection
							f_validation:writeln(string.format("\tdefault stage = %s [OK]", stripped_stage))
							f_table_insert(stages_selection, valid_path)
						elseif rc == 1 then
							f_table_insert(stages_selection, valid_path)
							modified_line = src_line:gsub(stripped_stage, valid_path)
						elseif rc == -1 then
							f_validation:writeln(string.format("\twith stage %s [NOT FOUND]", stripped_stage))
						elseif rc == -2 then
							f_validation:writeln(string.format("\twith stage %s [ERROR]", stripped_stage))
						end
					end
				end

				if string.find(stripped_ch, "%.[Dd][eE][fF]") then
					char_def = stripped_ch
				else
					char_def = stripped_ch .. "/" .. stripped_ch .. ".def"
				end

				valid_path, rc = findFile(char_def, { "", "chars" })
				if string.find(valid_path, '%.[Dd][eE][fF]') then
					table.insert(chars_selection, valid_path)
				end

				if rc == 0 then -- found
					f_validation:writeln(string.format("\t%s = %s [OK]", stripped_ch, valid_path))
				elseif rc == 1 then -- found in diff case, fixed
					f_validation:writeln(string.format("\t%s = %s [FIXED]", stripped_ch, valid_path))
					if modified_line == "" then
						modified_line = src_line:gsub(stripped_ch, valid_path, 1)
					else
						modified_line = modified_line:gsub(stripped_ch, valid_path, 1)
					end
				elseif rc == -1 then -- not found
					local status
					for k, v in ipairs(getDirectoryFiles("chars")) do
						if v:lower() == stripped_ch:lower() then
							if modified_line == "" then
								modified_line = src_line:gsub(stripped_ch, v, 1)
							else
								modified_line = modified_line:gsub(stripped_ch, v, 1)
							end
							table.insert(chars_selection, "chars/" .. v .. "/" .. v .. ".def")
						end
					end
				elseif rc == -2 then -- error happen
					f_validation:writeln(string.format("\t%s [ERROR]", stripped_ch))
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
				valid_path, rc = findFile(c, { "", "stages" })
				if rc == 0 then -- if found or fixed then add into stages_selection
					f_table_insert(stages_selection, valid_path)
					f_validation:writeln("\t" .. c .. " [OK]")
				elseif rc == 1 then
					f_table_insert(stages_selection, valid_path)
					modified_line = src_line:gsub(c, valid_path)
				elseif rc == -1 then
					f_validation:writeln("\t" .. c .. " [NOT FOUND]")
				elseif rc == -2 then
					f_validation:writeln("\t" .. c .. " [ERROR]")
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
			src_line = src_line:gsub('\\', '/')
			f_validation:writeln("\t[FIXED] " .. src_line)
		end
		file:write(src_line .. "\n")
	else
		if string.find(modified_line, '\\') then
			modified_line = modified_line:gsub('\\', '/')
		end
		f_validation:writeln("\t" .. modified_line .. " [FIXED]")
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
		f_validation:writeln("[ERROR] Can not read chars " .. ch)
	else
		f_validation:writeln("[select.def] " .. ch)

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
		if err ~= nil then
			print("DEBUG", ch, err)
		end
		for src_line in content:gmatch('([^\n]*)\n?') do
			line = src_line:gsub('%s*;.*$', '')
			if line:match('^[^%g]*%s*%[.-%s*%]%s*$') then --matched [] group
				line = line:match('%[(.-)%s*%]%s*$') --match text between []
				line = line:gsub('[%. ]', '_')   --change . and space to _
				group = tostring(line:lower())
			else                                 --matched non [] line
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
					value = value:gsub('"', '')       --remove brackets from value
					value = value:gsub('^(%.[0-9])', '0%1') --add 0 before dot if missing at the beginning of matched string
					value = value:gsub('([^0-9])(%.[0-9])', '%10%2') --add 0 before dot if missing anywhere else
					value = value:gsub(',%s*$', '')   --remove dummy ','
					if group == 'files' or group == 'arcade' then
						local rc = 99
						local charDir = ch:match(".*" .. sep)
						valid_path, rc = findFile(value, { charDir, "", "chars", "sound", motifDir, "data" })
						if rc == 0 then
							f_validation:writeln(string.format("\t[%s] %s = %s [OK]", group, param, value))
						elseif rc == 1 then
							modified_line = param .. " = " .. valid_path
						elseif rc == -1 then
							-- if NOT FOUND try check value dir's name
							local vdir = f_extractDir(value)
							if vdir ~= nil and vdir ~= "" then
								vdir = vdir:gsub("/$", "") -- remove trailing sep
								local dirs = getDirectoryFiles(charDir)
								for k, dir in ipairs(dirs) do
									if dir:lower() == vdir:lower() then
										modified_line = param .. " = " .. value:gsub(vdir, dir)
									end
								end
							end
							if modified_line == "" then
								f_validation:writeln(string.format("\t[%s] %s = %s [NOT FOUND]", group, param, value))
							end
						elseif rc == -2 then
							f_validation:writeln(string.format("\t[%s] %s = %s [ERROR]", group, param, value))
						end
					end
				end
			end
			if modified_line == "" then
				if string.find(src_line, '\\') then
					src_line = src_line:gsub('\\', '/')
					f_validation:writeln("\t[FIXED] " .. src_line)
				end
				file:write(src_line .. "\n")
			else
				if string.find(modified_line, '\\') then
					modified_line = modified_line:gsub('\\', '/')
				end
				f_validation:writeln("\t" .. modified_line .. " [FIXED]")
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
		f_validation:writeln("[ERROR] Can not read stage " .. stage)
	else
		f_validation:writeln("[select.def] " .. stage)

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
		for src_line in content:gmatch('([^\n]*)\n?') do
			line = src_line:gsub('%s*;.*$', '')
			if line:match('^[^%g]*%s*%[.-%s*%]%s*$') then --matched [] group
				line = line:match('%[(.-)%s*%]%s*$') --match text between []
				line = line:gsub('[%. ]', '_')   --change . and space to _
				group = tostring(line:lower())
			else                                 --matched non [] line
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
					stageDir = stage:match(".*" .. sep)
					if param:lower() == "spr" or param:lower() == "model" or param:lower() == "bgmusic" then
						local rc = 99
						valid_path, rc = findFile(value, { "", stageDir, "stages", motifDir, "data", "sound" })
						if rc == 0 then
							f_validation:writeln(string.format("\t[%s] %s = %s [OK]", group, param, value))
						elseif rc == 1 then
							modified_line = param .. " = " .. valid_path
						elseif rc == -1 then
							f_validation:writeln(string.format("\t[%s] %s = %s [NOT FOUND]", group, param, value))
						elseif rc == -2 then
							f_validation:writeln(string.format("\t[%s] %s = %s [ERROR]", group, param, value))
						end
					end
				end
			end
			if modified_line == "" then
				if string.find(src_line, '\\') then
					src_line = src_line:gsub('\\', '/')
					f_validation:writeln("\t[FIXED] " .. src_line)
				end
				file:write(src_line .. "\n")
			else
				if string.find(modified_line, '\\') then
					modified_line = modified_line:gsub('\\', '/')
				end
				f_validation:writeln("\t" .. modified_line .. " [FIXED]")
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
		f_validation:writeln("[ERROR] Can not read storyboard " .. sb)
	else
		f_validation:writeln("[system.def] Storyboard: " .. sb)

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
		for src_line in content:gmatch('([^\n]*)\n?') do
			local line = src_line:gsub('%s*;.*$', '')
			if line:match('^[^%g]*%s*%[.-%s*%]%s*$') then --matched [] group
				line = line:match('%[(.-)%s*%]%s*$') --match text between []
				line = line:gsub('[%. ]', '_')   --change . and space to _
				group = tostring(line:lower())
			else                                 --matched non [] line
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
					local rc = 99
					param = param:lower()
					value = value:gsub('"', '') --remove brackets from value
					sbDir = sb:match(".*" .. sep)
					if param == "spr" or param == "snd" or param == "bgm" then
						valid_path, rc = findFile(value, { "", sbDir, motifDir, "font", "sound" })
						if rc == 0 then
							f_validation:writeln(string.format("\t[%s] %s = %s [OK]", group, param, value))
						elseif rc == 1 then
							modified_line = param .. " = " .. valid_path
						elseif rc == -1 then
							f_validation:writeln(string.format("\t[%s] %s = %s [NOT FOUND]", group, param, value))
						elseif rc == -2 then
							f_validation:writeln(string.format("\t[%s] %s = %s [ERROR]", group, param, value))
						end
					elseif param:find("font[0-9]+") then
						valid_path, rc = findFile(value, { "", sbDir, "font/", motifDir })
						if rc == 0 then
							f_validation:writeln(string.format("\t[%s] %s = %s [OK]", group, param, value))
						elseif rc == 1 then
							modified_line = param .. " = " .. valid_path
						elseif rc == -1 then
							f_validation:writeln(string.format("\t[%s] %s = %s [NOT FOUND]", group, param, value))
						elseif rc == -2 then
							f_validation:writeln(string.format("\t[%s] %s = %s [ERROR]", group, param, value))
						end
						if string.find(valid_path, '%.[Dd][eE][fF]') then
							table.insert(fonts_selection, valid_path)
						end
					end
				end
			end
			if modified_line == "" then
				if string.find(src_line, '\\') and not string.find(src_line, 'text') then
					src_line = src_line:gsub('\\', '/')
					f_validation:writeln("\t[FIXED] " .. src_line)
				end
				file:write(src_line .. "\n")
			else
				if string.find(modified_line, '\\') then
					modified_line = modified_line:gsub('\\', '/')
				end
				f_validation:writeln("\t" .. modified_line .. " [FIXED]")
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
		f_validation:writeln("[ERROR] Can not read font " .. font)
	else
		f_validation:writeln("[system.def] Font: " .. font)

		local group
		local sep

		if string.find(font, '\\') then
			sep = '\\'
		else
			sep = '/'
		end

		local file, err = io.open(font, "w")
		local fontDir = f_extractDir(font)
		for src_line in content:gmatch('([^\n]*)\n?') do
			line = src_line:gsub('%s*;.*$', '')
			if line:match('^[^%g]*%s*%[.-%s*%]%s*$') then --matched [] group
				line = line:match('%[(.-)%s*%]%s*$') --match text between []
				line = line:gsub('[%. ]', '_')   --change . and space to _
				group = tostring(line:lower())
			else                                 --matched non [] line
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
					local rc = 99
					param = param:lower()
					value = value:gsub('"', '') --remove brackets from value
					if param == "file" then
						valid_path, rc = findFile(value, { "", fontDir, "fonts", "data" })
						if rc == 0 then
							f_validation:writeln(string.format("\t[%s] %s = %s [OK]", group, param, value))
						elseif rc == 1 then
							modified_line = param .. " = " .. valid_path
						elseif rc == -1 then
							f_validation:writeln(string.format("\t[%s] %s = %s [NOT FOUND]", group, param, value))
						elseif rc == -2 then
							f_validation:writeln(string.format("\t[%s] %s = %s [ERROR]", group, param, value))
						end
					end
				end
			end
			if modified_line == "" then
				if string.find(src_line, '\\') then
					src_line = src_line:gsub('\\', '/')
					f_validation:writeln("\t[FIXED] " .. src_line)
				end
				file:write(src_line .. "\n")
			else
				if string.find(modified_line, '\\') then
					modified_line = modified_line:gsub('\\', '/')
				end
				f_validation:writeln("\t" .. modified_line .. " [FIXED]")
				file:write(modified_line .. "\n")
			end
			modified_line = ""
		end
		file:close()
	end
end
f_validation:close()
