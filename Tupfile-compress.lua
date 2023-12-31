tup.include("Tupfile-consts.lua")

files = tup.glob("data/*_s?_wkb.gpkg")
output_dir = "data/"
minprecxy = 3

for i = 1, #files do
  input = files[i]
  fullbase = tup.base(input)
  base = fullbase:sub(1, -8)
  simplify = fullbase:sub(-6, -5)
  sqltable = basetotablemap[base] or base

  -- -- Compress to TWKB
  inputs = {"$(tgpkg)", input}
  output = output_dir .. base .. "_" .. simplify .. "_twkb_p" .. minprecxy .. ".gpkg"
  cmd = '^s^ $(tgpkg) ' ..
  ' -table ' .. table_from_base(base) ..
  ' -minprecxy ' .. minprecxy ..
  ' -o ' .. output ..
  ' compress ' .. input

  tup.rule(inputs, cmd, output)

  twkb_output = output

  -- Decompress TWKB as a roundtrip test
  inputs = {"$(tgpkg)", twkb_output}
  output = output_dir .. base .. "_roundtrip_" .. simplify .. "_twkb_p" .. minprecxy .. ".gpkg"
  cmd = '^s^ $(tgpkg) ' ..
  ' -table ' .. table_from_base(base) ..
  ' -minprecxy ' .. minprecxy ..
  ' -o ' .. output ..
  ' decompress ' .. twkb_output

  tup.rule(inputs, cmd, output)
end
