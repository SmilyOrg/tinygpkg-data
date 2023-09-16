files = tup.glob("data/*_roundtrip_*.gpkg")
output_dir = "data/"

function gdal(cmd)
  return 'docker run --rm -i' ..
  ' -w / ghcr.io/osgeo/gdal:alpine-normal-3.7.1 ' ..
  cmd
end

places = {
  {"world", 0, 0, 0},
  {"europe", 3, 49, 9.6},
  {"africa", 2, 6, 19},
  {"usa", 3, 40, -101},
  {"japan", 5, 35, 130},
}


for i = 1, #files do
  input = files[i]
  filename = tup.file(input)
  fullbase = tup.base(input)
  input = output_dir .. filename
  base = fullbase:sub(1, -22)

  w, h = 1024, 1024
  multisample = 1
  mw, mh = w * multisample, h * multisample
  
  for i = 1, #places do
    local place = places[i]
    local name = place[1]
    local level = place[2]
    local lat = place[3]
    local lng = place[4]

    local ar = h / w * 360 / 180
    local p = 2^level
    local cx, cy = lng, lat
    local xs = 360 / p * 0.5
    local xmin, xmax = cx-xs, cx+xs
    local ys = 180 / p * 0.5
    local ymin, ymax = cy-ys*ar, cy+ys*ar
    
    output = output_dir .. fullbase .. "_" .. name .. ".tiff"
    outputpng = output_dir .. fullbase .. "_" .. name .. ".png"

    cmd = '^s^ ' ..
    'cat ' .. input .. ' | ' ..
    gdal(
      'sh -c "' ..
      'cp /dev/stdin input.gpkg &&' ..
      'gdal_rasterize ' ..
      ' -q ' ..
      ' -init 255 ' ..
      ' -burn 90 ' ..
      ' -ot Byte ' ..
      ' -ts ' .. mw .. ' ' .. mh .. ' ' ..
      ' -te ' .. xmin .. ' ' .. ymin .. ' ' .. xmax .. ' ' .. ymax .. ' ' ..
      ' -l ' .. base .. ' ' ..
      ' input.gpkg ' ..
      ' /vsistdout/ | ' ..
      'gdal_translate ' ..
      ' -q ' ..
      ' -of PNG ' ..
      ' -r lanczos ' ..
      ' -outsize ' .. w .. ' ' .. h .. ' ' ..
      ' /vsistdin/ ' ..
      ' /vsistdout/ ' ..
      '"'
    ) ..
    ' > ' .. outputpng ..
    ' && optipng -quiet ' .. outputpng

    tup.rule(input, cmd, outputpng)
  end
end