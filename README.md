<!-- Generated from README.tmpl.md DO NOT EDIT -->

# Tiny GeoPackage

This repository contains a set of scripts and tools for generating compressed GeoPackage files from various open data sources.

## Datasets



| Name                          | Contents                                | Features | Source          | License                            |
| ----------------------------- | --------------------------------------- | -------: | --------------- | ---------------------------------- |
| [ne_110m_admin_0_countries]   | Country borders, 1:110m scale           |      177 | [Natural Earth] | [Public Domain][ne-license]        |
| [ne_10m_admin_0_countries]    | Country borders, 1:10m scale            |      258 | [Natural Earth] | [Public Domain][ne-license]        |
| [ne_10m_urban_areas_landscan] | Big cities only, 1:10m scale            |     6018 | [Natural Earth] | [Public Domain][ne-license]        |
| [geoBoundariesCGAZ_ADM0]      | Country-level administrative boundaries |      200 | [geoBoundaries] | [Attribution required][gb-license] |
| [geoBoundariesCGAZ_ADM2]      | City-level administrative boundaries    |    49689 | [geoBoundaries] | [Attribution required][gb-license] |


[ne_110m_admin_0_countries]: #ne_110m_admin_0_countries
[ne_10m_admin_0_countries]: #ne_10m_admin_0_countries
[ne_10m_urban_areas_landscan]: #ne_10m_urban_areas_landscan
[geoBoundariesCGAZ_ADM0]: #geoboundariescgaz_adm0
[geoBoundariesCGAZ_ADM2]: #geoboundariescgaz_adm2



[Natural Earth]: https://www.naturalearthdata.com/
[geoBoundaries]: https://www.geoboundaries.org
[ne-license]: https://www.naturalearthdata.com/about/terms-of-use/
[gb-license]: https://www.geoboundaries.org/index.html#citation

## Parameters

Source datasets are compressed using two methods, simplification and [Tiny
Well-known Binary (TWKB)][TWKB] compression.

Simplification is performed using the Ramer-Douglas-Peucker [Simplify] method on
the polygons. If the simplification fails (creates an invalid polygon), less and
less simplification is used until the polygon remains valid. If the polygon has
less than "Min. Points", it is not simplified.

Precision is the maximum number of decimal places used to store the coordinates
using [TWKB]. From empirical testing, less than 3 decimal places does not save a
lot of space and more than 3 decimal places does not gain a lot in precision for
these datasets.

| Name | Simplify | Min. Points | Precision |
| --- | --- | --- | --- |


| Name       | Simplify | Min. Points | Precision |
| ---------- | -------- | ----------- | --------- |
| s3_twkb_p3 | 1        | 20          | 3         |
| s4_twkb_p3 | 0.1      | 20          | 3         |
| s5_twkb_p3 | 0.01     | 20          | 3         |
| s6_twkb_p3 | 0.001    | 20          | 3         |
| s7_twkb_p3 | 0.0001   | 20          | 3         |
| s8_twkb_p3 | 0.00001  | 20          | 3         |

[TWKB]: https://github.com/TWKB/Specification/blob/master/twkb.md
[Simplify]: https://pkg.go.dev/github.com/peterstace/simplefeatures/geom#Geometry.Simplify

## Variants

See [Parameters](#parameters) for details on the parameters used for each variant.






### ne_110m_admin_0_countries

See [Parameters](#parameters) for what each variant means and
[Datasets](#datasets) for details on the dataset itself.

| Variant | Size |  europe |  africa |  usa |  japan |  world | 
| --- | --- |  --- |  --- |  --- |  --- |  --- | 
| [s3_twkb_p3](data/ne_110m_admin_0_countries_s3_twkb_p3.gpkg) | 352 KB | <a href="data/ne_110m_admin_0_countries_roundtrip_s3_twkb_p3_world.png"><img src="data/ne_110m_admin_0_countries_roundtrip_s3_twkb_p3_world.png" alt="ne_110m_admin_0_countries s3_twkb_p3 world"></a> |<a href="data/ne_110m_admin_0_countries_roundtrip_s3_twkb_p3_europe.png"><img src="data/ne_110m_admin_0_countries_roundtrip_s3_twkb_p3_europe.png" alt="ne_110m_admin_0_countries s3_twkb_p3 europe"></a> |<a href="data/ne_110m_admin_0_countries_roundtrip_s3_twkb_p3_africa.png"><img src="data/ne_110m_admin_0_countries_roundtrip_s3_twkb_p3_africa.png" alt="ne_110m_admin_0_countries s3_twkb_p3 africa"></a> |<a href="data/ne_110m_admin_0_countries_roundtrip_s3_twkb_p3_usa.png"><img src="data/ne_110m_admin_0_countries_roundtrip_s3_twkb_p3_usa.png" alt="ne_110m_admin_0_countries s3_twkb_p3 usa"></a> |<a href="data/ne_110m_admin_0_countries_roundtrip_s3_twkb_p3_japan.png"><img src="data/ne_110m_admin_0_countries_roundtrip_s3_twkb_p3_japan.png" alt="ne_110m_admin_0_countries s3_twkb_p3 japan"></a> |
| [s4_twkb_p3](data/ne_110m_admin_0_countries_s4_twkb_p3.gpkg) | 393 KB | <a href="data/ne_110m_admin_0_countries_roundtrip_s4_twkb_p3_world.png"><img src="data/ne_110m_admin_0_countries_roundtrip_s4_twkb_p3_world.png" alt="ne_110m_admin_0_countries s4_twkb_p3 world"></a> |<a href="data/ne_110m_admin_0_countries_roundtrip_s4_twkb_p3_europe.png"><img src="data/ne_110m_admin_0_countries_roundtrip_s4_twkb_p3_europe.png" alt="ne_110m_admin_0_countries s4_twkb_p3 europe"></a> |<a href="data/ne_110m_admin_0_countries_roundtrip_s4_twkb_p3_africa.png"><img src="data/ne_110m_admin_0_countries_roundtrip_s4_twkb_p3_africa.png" alt="ne_110m_admin_0_countries s4_twkb_p3 africa"></a> |<a href="data/ne_110m_admin_0_countries_roundtrip_s4_twkb_p3_usa.png"><img src="data/ne_110m_admin_0_countries_roundtrip_s4_twkb_p3_usa.png" alt="ne_110m_admin_0_countries s4_twkb_p3 usa"></a> |<a href="data/ne_110m_admin_0_countries_roundtrip_s4_twkb_p3_japan.png"><img src="data/ne_110m_admin_0_countries_roundtrip_s4_twkb_p3_japan.png" alt="ne_110m_admin_0_countries s4_twkb_p3 japan"></a> |
| [s5_twkb_p3](data/ne_110m_admin_0_countries_s5_twkb_p3.gpkg) | 393 KB | <a href="data/ne_110m_admin_0_countries_roundtrip_s5_twkb_p3_world.png"><img src="data/ne_110m_admin_0_countries_roundtrip_s5_twkb_p3_world.png" alt="ne_110m_admin_0_countries s5_twkb_p3 world"></a> |<a href="data/ne_110m_admin_0_countries_roundtrip_s5_twkb_p3_europe.png"><img src="data/ne_110m_admin_0_countries_roundtrip_s5_twkb_p3_europe.png" alt="ne_110m_admin_0_countries s5_twkb_p3 europe"></a> |<a href="data/ne_110m_admin_0_countries_roundtrip_s5_twkb_p3_africa.png"><img src="data/ne_110m_admin_0_countries_roundtrip_s5_twkb_p3_africa.png" alt="ne_110m_admin_0_countries s5_twkb_p3 africa"></a> |<a href="data/ne_110m_admin_0_countries_roundtrip_s5_twkb_p3_usa.png"><img src="data/ne_110m_admin_0_countries_roundtrip_s5_twkb_p3_usa.png" alt="ne_110m_admin_0_countries s5_twkb_p3 usa"></a> |<a href="data/ne_110m_admin_0_countries_roundtrip_s5_twkb_p3_japan.png"><img src="data/ne_110m_admin_0_countries_roundtrip_s5_twkb_p3_japan.png" alt="ne_110m_admin_0_countries s5_twkb_p3 japan"></a> |
| [s6_twkb_p3](data/ne_110m_admin_0_countries_s6_twkb_p3.gpkg) | 393 KB | <a href="data/ne_110m_admin_0_countries_roundtrip_s6_twkb_p3_world.png"><img src="data/ne_110m_admin_0_countries_roundtrip_s6_twkb_p3_world.png" alt="ne_110m_admin_0_countries s6_twkb_p3 world"></a> |<a href="data/ne_110m_admin_0_countries_roundtrip_s6_twkb_p3_europe.png"><img src="data/ne_110m_admin_0_countries_roundtrip_s6_twkb_p3_europe.png" alt="ne_110m_admin_0_countries s6_twkb_p3 europe"></a> |<a href="data/ne_110m_admin_0_countries_roundtrip_s6_twkb_p3_africa.png"><img src="data/ne_110m_admin_0_countries_roundtrip_s6_twkb_p3_africa.png" alt="ne_110m_admin_0_countries s6_twkb_p3 africa"></a> |<a href="data/ne_110m_admin_0_countries_roundtrip_s6_twkb_p3_usa.png"><img src="data/ne_110m_admin_0_countries_roundtrip_s6_twkb_p3_usa.png" alt="ne_110m_admin_0_countries s6_twkb_p3 usa"></a> |<a href="data/ne_110m_admin_0_countries_roundtrip_s6_twkb_p3_japan.png"><img src="data/ne_110m_admin_0_countries_roundtrip_s6_twkb_p3_japan.png" alt="ne_110m_admin_0_countries s6_twkb_p3 japan"></a> |
| [s7_twkb_p3](data/ne_110m_admin_0_countries_s7_twkb_p3.gpkg) | 393 KB | <a href="data/ne_110m_admin_0_countries_roundtrip_s7_twkb_p3_world.png"><img src="data/ne_110m_admin_0_countries_roundtrip_s7_twkb_p3_world.png" alt="ne_110m_admin_0_countries s7_twkb_p3 world"></a> |<a href="data/ne_110m_admin_0_countries_roundtrip_s7_twkb_p3_europe.png"><img src="data/ne_110m_admin_0_countries_roundtrip_s7_twkb_p3_europe.png" alt="ne_110m_admin_0_countries s7_twkb_p3 europe"></a> |<a href="data/ne_110m_admin_0_countries_roundtrip_s7_twkb_p3_africa.png"><img src="data/ne_110m_admin_0_countries_roundtrip_s7_twkb_p3_africa.png" alt="ne_110m_admin_0_countries s7_twkb_p3 africa"></a> |<a href="data/ne_110m_admin_0_countries_roundtrip_s7_twkb_p3_usa.png"><img src="data/ne_110m_admin_0_countries_roundtrip_s7_twkb_p3_usa.png" alt="ne_110m_admin_0_countries s7_twkb_p3 usa"></a> |<a href="data/ne_110m_admin_0_countries_roundtrip_s7_twkb_p3_japan.png"><img src="data/ne_110m_admin_0_countries_roundtrip_s7_twkb_p3_japan.png" alt="ne_110m_admin_0_countries s7_twkb_p3 japan"></a> |
| [s8_twkb_p3](data/ne_110m_admin_0_countries_s8_twkb_p3.gpkg) | 393 KB | <a href="data/ne_110m_admin_0_countries_roundtrip_s8_twkb_p3_world.png"><img src="data/ne_110m_admin_0_countries_roundtrip_s8_twkb_p3_world.png" alt="ne_110m_admin_0_countries s8_twkb_p3 world"></a> |<a href="data/ne_110m_admin_0_countries_roundtrip_s8_twkb_p3_europe.png"><img src="data/ne_110m_admin_0_countries_roundtrip_s8_twkb_p3_europe.png" alt="ne_110m_admin_0_countries s8_twkb_p3 europe"></a> |<a href="data/ne_110m_admin_0_countries_roundtrip_s8_twkb_p3_africa.png"><img src="data/ne_110m_admin_0_countries_roundtrip_s8_twkb_p3_africa.png" alt="ne_110m_admin_0_countries s8_twkb_p3 africa"></a> |<a href="data/ne_110m_admin_0_countries_roundtrip_s8_twkb_p3_usa.png"><img src="data/ne_110m_admin_0_countries_roundtrip_s8_twkb_p3_usa.png" alt="ne_110m_admin_0_countries s8_twkb_p3 usa"></a> |<a href="data/ne_110m_admin_0_countries_roundtrip_s8_twkb_p3_japan.png"><img src="data/ne_110m_admin_0_countries_roundtrip_s8_twkb_p3_japan.png" alt="ne_110m_admin_0_countries s8_twkb_p3 japan"></a> |


