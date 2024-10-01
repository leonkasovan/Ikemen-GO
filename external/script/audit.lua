-- Lua script for verifying Ikemen's game assets
-- Dhani.Novan@gmail.com
-- 20.10 Selasa, 01 Oktober 2024

--One-time load of the json routines
json = (loadfile 'external/script/json.lua')()

--;===========================================================
--; COMMON FUNCTIONS
--;===========================================================

--return file content
function f_fileRead(path, mode)
	local file = io.open(path, mode or 'r')
	if file == nil then
		panicError("\nFile doesn't exist: " .. path)
		return
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
					if file == "stages/bmoo.sff" then
						print("debug", value..file)
					end
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
end

-------------------------------------------------------------------
-- CHECK config.json
-------------------------------------------------------------------
local fonts_selection = {}
local content
local config_file = "save/config.json"
local config = json.decode(f_fileRead(config_file))
if config == nil then
	print("[ERROR] Can not load "..config_file)
	return
end

f_checkFile(config_file, "Ikemen Config")
for index, value in ipairs(config.CommonAir) do
	f_checkFile(value, "[config.json] CommonAir["..tostring(index).."]")
end
for index, value in ipairs(config.CommonCmd) do
	f_checkFile(value, "[config.json] CommonCmd["..tostring(index).."]")
end
for index, value in ipairs(config.CommonConst) do
	f_checkFile(value, "[config.json] CommonConst["..tostring(index).."]")
end
for index, value in ipairs(config.CommonStates) do
	f_checkFile(value, "[config.json] CommonStates["..tostring(index).."]")
end

f_checkFile(config.DebugFont, "[config.json] DebugFont")
table.insert(fonts_selection, config.DebugFont)
f_checkFile(config.Motif, "[config.json] Motif")
local motifDir = config.Motif:match('^(.-)[^/\\]+$')
print(string.format('[config.json] Motif Directory: %s', motifDir))
f_checkFile(config.StartStage, "[config.json] StartStage")
f_checkFile(config.System, "[config.json] System")

for index, value in ipairs(config.WindowIcon) do
	f_checkFile(value, "[config.json] WindowIcon["..tostring(index).."]")
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

for line in content:gmatch('([^\n]*)\n?') do
	line = line:gsub('%s*;.*$', '')
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
					table.insert(fonts_selection, motif[param])
				else
					motif[param] = searchFile(value, {motifDir, "data/"})
				end
				f_checkFile(motif[param], "[system.def] "..param)
			end
			
		end
	end
end

-------------------------------------------------------------------
-- CHECK motif.select: select.def
-------------------------------------------------------------------
content = f_fileRead(motif.select)
if content == nil then
	print("[ERROR] Can not read "..motif.select)
	return
end
content = content:gsub('([^\r\n;]*)%s*;[^\r\n]*', '%1')
content = content:gsub('\n%s*\n', '\n')

local lanChars = false
local lanStages = false
local lanOptions = false
local lanStory = false
for line in content:gmatch('[^\r\n]+') do
	local lineCase = line:lower()
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

for line in content:gmatch('[^\r\n]+') do
	local lineCase = line:lower()
	if lineCase:match('^%s*%[%s*characters%s*%]') then
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
		-- main.t_selOptions = {
			-- arcadestart = {wins = 0, offset = 0},
			-- arcadeend = {wins = 0, offset = 0},
			-- teamstart = {wins = 0, offset = 0},
			-- teamend = {wins = 0, offset = 0},
			-- survivalstart = {wins = 0, offset = 0},
			-- survivalend = {wins = 0, offset = 0},
			-- ratiostart = {wins = 0, offset = 0},
			-- ratioend = {wins = 0, offset = 0},
		-- }
		row = 0
		section = 3
	elseif lineCase:match('^%s*%[%s*' .. config.Language .. '.options' .. '%s*%]') then
		print("[select.def]"..line)
		if lanOptions then
			-- main.t_selOptions = {
				-- arcadestart = {wins = 0, offset = 0},
				-- arcadeend = {wins = 0, offset = 0},
				-- teamstart = {wins = 0, offset = 0},
				-- teamend = {wins = 0, offset = 0},
				-- survivalstart = {wins = 0, offset = 0},
				-- survivalend = {wins = 0, offset = 0},
				-- ratiostart = {wins = 0, offset = 0},
				-- ratioend = {wins = 0, offset = 0},
			-- }
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
		-- local csCol = (csCell % motif.select_info.columns) + 1
		-- local csRow = math.floor(csCell / motif.select_info.columns) + 1
		-- while not slot and motif.select_info['cell_' .. csCol .. '_' .. csRow .. '_skip'] == 1 do
			-- main.f_addChar('skipslot', true, true, false)
			-- csCell = csCell + 1
			-- csCol = (csCell % motif.select_info.columns) + 1
			-- csRow = math.floor(csCell / motif.select_info.columns) + 1
		-- end
		-- if lineCase:match(',%s*exclude%s*=%s*1') then --character should be added after all slots are filled
			-- print("302", lineCase, line)
			-- table.insert(t_addExluded, line)
		if lineCase:match('^%s*slot%s*=%s*{%s*$') then --start of the 'multiple chars in one slot' assignment
			-- print("305", lineCase)
			-- table.insert(main.t_selGrid, {['chars'] = {}, ['slot'] = 1})
			-- slot = true
		elseif slot and lineCase:match('^%s*}%s*$') then --end of 'multiple chars in one slot' assignment
			print("309", lineCase)
			-- slot = false
			-- csCell = csCell + 1
		else
			-- print("313", line)
			if line ~= "randomselect" and line ~= "blank" and line ~= "}" then
				local char_found
				local c = f_strsplit(',', line)
				local stripped_ch = c[1]:match("^%s*(.-)%s*$")
				
				if string.find(stripped_ch, ".def") then
					char_found = searchFile(stripped_ch, {motifDir, "chars/"})
				else
					char_found = searchFile(stripped_ch.."/"..stripped_ch..".def", {motifDir, "chars/"})
				end
				f_checkFile(char_found, "\t"..stripped_ch)
				table.insert(chars_selection, char_found)
				-- f_addChar(line, true, true, slot)
				-- if not slot then
					-- csCell = csCell + 1
				-- end
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
				-- print("extrastages1", c)
				-- local stage_found = searchFile(c, {"./", motifDir, "stages/"})
				f_checkFile(c, "\t")
				table.insert(stages_selection, c)
				-- row = main.f_addStage(c, hidden)
				-- if row == nil then
					-- break
				-- end
				-- table.insert(main.t_includeStage[1], row)
				-- table.insert(main.t_includeStage[2], row)
			elseif c:match('^music') then --musicX / musiclife / musicvictory
				print("extrastages2", c)
				-- local bgmvolume, bgmloopstart, bgmloopend = 100, 0, 0
				-- c = c:gsub('%s+([0-9%s]+)$', function(m1)
					-- for i, c in ipairs(main.f_strsplit('%s+', m1)) do --split using whitespace delimiter
						-- if i == 1 then
							-- bgmvolume = tonumber(c)
						-- elseif i == 2 then
							-- bgmloopstart = tonumber(c)
						-- elseif i == 3 then
							-- bgmloopend = tonumber(c)
						-- else
							-- break
						-- end
					-- end
					-- return ''
				-- end)
				-- c = c:gsub('\\', '/')
				-- local bgtype, round, bgmusic = c:match('^(music[a-z]*)([0-9]*)%s*=%s*(.-)%s*$')
				-- if main.t_selStages[row][bgtype] == nil then main.t_selStages[row][bgtype] = {} end
				-- local t_ref = main.t_selStages[row][bgtype]
				-- if bgtype == 'music' or round ~= '' then
					-- round = tonumber(round) or 1
					-- if main.t_selStages[row][bgtype][round] == nil then main.t_selStages[row][bgtype][round] = {} end
					-- t_ref = main.t_selStages[row][bgtype][round]
				-- end
				-- table.insert(t_ref, {bgmusic = bgmusic, bgmvolume = bgmvolume, bgmloopstart = bgmloopstart, bgmloopend = bgmloopend})
			else
				print("extrastages3", c)
				local param, value = c:match('^(.-)%s*=%s*(.-)$')
				if param ~= nil and value ~= nil and param ~= '' and value ~= '' then
					-- main.t_selStages[row][param] = tonumber(value)
					-- order (more than 1 order param can be set at the same time)
					-- if param:match('order') then
						-- if main.t_orderStages[main.t_selStages[row].order] == nil then
							-- main.t_orderStages[main.t_selStages[row].order] = {}
						-- end
						-- table.insert(main.t_orderStages[main.t_selStages[row].order], row)
					-- end
					print("extrastages3 param, value", param, value)
				end
			end
			--default order
			-- if main.t_selStages[row].order == nil then
				-- main.t_selStages[row].order = 1
				-- if main.t_orderStages[main.t_selStages[row].order] == nil then
					-- main.t_orderStages[main.t_selStages[row].order] = {}
				-- end
				-- table.insert(main.t_orderStages[main.t_selStages[row].order], row)
			-- end
			--unlock param
			-- if unlock ~= '' then
				--main.t_selStages[row].unlock = unlock
				-- main.t_unlockLua.stages[row] = unlock
			-- end
		end
	elseif section == 3 then --[Options]
		-- skip
	elseif section == 4 then --[StoryMode]
		-- skip
	end
end

-------------------------------------------------------------------
-- CHECK Characters: chars/*/*.def
-------------------------------------------------------------------
for i, ch in ipairs(chars_selection) do
	content = f_fileRead(ch)
	if content == nil then
		print("[ERROR] Can not read chars "..ch)
		return
	end
	print("[select.def] "..ch)

	local group
	local charDir
	local sep
	
	if string.find(ch, '\\') then
		sep = '\\'
	else
		sep = '/'
	end

	for line in content:gmatch('([^\n]*)\n?') do
		line = line:gsub('%s*;.*$', '')
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
				
				if group == 'files' or group == 'arcade'then
					charDir = ch:match(".*"..sep)
					-- print("value", value)
					-- print("charDir", charDir)
					-- f_checkFile(searchFile(value, {charDir, "chars/"}), "\t"..param)
					f_checkFile(value, "\t"..param, {charDir, "chars"..sep, "data"..sep})
				end
				
			end
		end
	end
end

-------------------------------------------------------------------
-- CHECK Stages: stages/*.def
-------------------------------------------------------------------
for index, stage in ipairs(stages_selection) do
	content = f_fileRead(stage)
	if content == nil then
		print("[ERROR] Can not read chars "..stage)
		return
	end
	print("[select.def] "..stage)

	local group
	local stageDir
	local sep
	
	if string.find(stage, '\\') then
		sep = '\\'
	else
		sep = '/'
	end

	for line in content:gmatch('([^\n]*)\n?') do
		line = line:gsub('%s*;.*$', '')
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
				if param == "spr" or param == "model" or param == "bgMusic" then
					f_checkFile(value, "\t"..param, {stageDir, "stages"..sep, "data"..sep})
				end
			end
		end
	end
end

-------------------------------------------------------------------
-- CHECK Fonts: fonts/*.def
-------------------------------------------------------------------
for index, font in ipairs(fonts_selection) do
	if string.find(font, '.def') then
		content = f_fileRead(font)
		if content == nil then
			print("[ERROR] Can not read chars "..font)
			return
		end
		print("[system.def] "..font)

		local group
		local fontDir
		local sep
		
		if string.find(font, '\\') then
			sep = '\\'
		else
			sep = '/'
		end

		for line in content:gmatch('([^\n]*)\n?') do
			line = line:gsub('%s*;.*$', '')
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
					fontDir = font:match(".*"..sep)
					if param == "file" then
						f_checkFile(value, "\t"..param, {fontDir, "fonts"..sep, "data"..sep})
					end
				end
			end
		end
	else
		print("non DEF file font: "..font)
	end
end

