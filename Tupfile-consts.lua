
basetotablemap = {
  geoBoundariesCGAZ_ADM2 = "globalADM2",
  geoBoundariesCGAZ_ADM0 = "globalADM0",
}

function table_from_base(base)
  return basetotablemap[base] or base
end
