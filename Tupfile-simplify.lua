tup.include("Tupfile-consts.lua")

files = tup.glob("data/*_makevalid.gpkg")
output_dir = "data/"
min_level = 3
max_level = 8

function generate_levels(n, m)
  local levels = {}
  for i = n, m do
    table.insert(levels, string.format('%.6f', 10^(3 - i)))
  end
  return table.concat(levels, ",")
end

for i = 1, #files do
  input = files[i]
  fullbase = tup.base(input)
  base = fullbase:gsub("%_makevalid", "")
  for j = min_level, max_level do
    inputs = {"$(simplify)", input}
    levels = generate_levels(j, 8)
    output = output_dir .. base .. "_s" .. j .. ".gpkg"
    cmd =
      '^s^ $(simplify) ' ..
      ' -table ' .. table_from_base(base) ..
      ' -levels ' .. levels ..
      ' -minpoints $(simplify_minpoints) ' ..
      ' -o ' .. output ..
      ' ' .. input
    tup.rule(inputs, cmd, output)
  end
end
