files = tup.glob("data/*_s?.gpkg")
output_dir = "data/"
minprecxy = 3

for i = 1, #files do
  input = files[i]
  fullbase = tup.base(input)
  base = fullbase:sub(1, -4)
  suffix = fullbase:sub(-3, -1)
  
  -- Compress to TWKB
  inputs = {"$(tgpkg)", input}
  output = output_dir .. fullbase .. "_twkb_p" .. minprecxy .. ".gpkg"
  cmd = '^s^ $(tgpkg) ' ..
  ' -table ' .. base ..
  ' -minprecxy ' .. minprecxy ..
  ' -o ' .. output ..
  ' compress ' .. input

  tup.rule(inputs, cmd, output)

  twkb_output = output

  -- Decompress TWKB as a roundtrip test
  inputs = {"$(tgpkg)", twkb_output}
  output = output_dir .. base .. "_roundtrip" .. suffix .. "_twkb_p" .. minprecxy .. ".gpkg"
  cmd = '^s^ $(tgpkg) ' ..
  ' -table ' .. base ..
  ' -minprecxy ' .. minprecxy ..
  ' -o ' .. output ..
  ' decompress ' .. twkb_output

  tup.rule(inputs, cmd, output)
end
