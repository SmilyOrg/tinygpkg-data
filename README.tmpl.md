# Tiny GeoPackage

This repository contains a set of scripts and tools for generating compressed GeoPackage files from various open data sources.

## Datasets

<!-- ### [Natural Earth](https://www.naturalearthdata.com/) ([Public Domain](https://www.naturalearthdata.com/about/terms-of-use/))

* [ne_110m_admin_0_countries] - country borders at 1:110m scale
* [ne_10m_admin_0_countries] - country borders at 1:10m scale
* [ne_10m_urban_areas_landscan] - cities at 1:10m scale

[ne_110m_admin_0_countries]: https://www.naturalearthdata.com/downloads/110m-cultural-vectors/
[ne_10m_admin_0_countries]: https://www.naturalearthdata.com/downloads/10m-cultural-vectors/

### [geoBoundaries](https://www.geoboundaries.org) ([Attribution Required](https://www.geoboundaries.org/index.html#citation))

Administrative boundaries courtesy of [geoBoundaries](https://www.geoboundaries.org).

Currently includes the [Comprehensive Global Administrative Zones (CGAZ)], but none of the individual country boundaries.

[Comprehensive Global Administrative Zones (CGAZ)]: https://www.geoboundaries.org/downloadCGAZ.html

* geoBoundariesCGAZ_ADM2 - roughly city-level detail
* geoBoundariesCGAZ_ADM1 - roughly state-level detail
* geoBoundariesCGAZ_ADM0 - country-level detail -->

| Name | Contents | Features | Source | License |
| --- | --- | ---: | --- | --- |
| [ne_110m_admin_0_countries] | Country borders, 1:110m scale | 177 | [Natural Earth] | [Public Domain][ne-license] |
| [ne_10m_admin_0_countries] | Country borders, 1:10m scale | 258 | [Natural Earth] | [Public Domain][ne-license] |
| [ne_10m_urban_areas_landscan] | Big cities only, 1:10m scale | 6 018 | [Natural Earth] | [Public Domain][ne-license] |
| [geoBoundariesCGAZ_ADM0] | Country-level administrative boundaries | 200 | [geoBoundaries] | [Attribution required][gb-license] |
| [geoBoundariesCGAZ_ADM2] | City-level administrative boundaries | 49 689 | [geoBoundaries] | [Attribution required][gb-license] |


[ne_110m_admin_0_countries]: #ne_110m_admin_0_countries
[ne_10m_admin_0_countries]: #ne_10m_admin_0_countries
[ne_10m_urban_areas_landscan]: #ne_10m_urban_areas_landscan
[geoBoundariesCGAZ_ADM0]: #geoboundariescgaz_adm0
[geoBoundariesCGAZ_ADM2]: #geoboundariescgaz_adm2

<!-- | geoBoundariesCGAZ_ADM1 | State-level administrative boundaries | [geoBoundaries] | [Attribution required][gb-license] | -->

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
{{range .Parameters -}}
| {{.Name}} | {{index .Simplify 0}} | {{ .SimplifyMinPoints }} | {{.MinPrecXY}} |
{{end}}

[TWKB]: https://github.com/TWKB/Specification/blob/master/twkb.md
[Simplify]: https://pkg.go.dev/github.com/peterstace/simplefeatures/geom#Geometry.Simplify

## Variants

See [Parameters](#parameters) for details on the parameters used for each variant.

{{range .Datasets}}

{{$dataset := .}}
{{$places := .Render.Places}}

### {{.Name}}

See [Parameters](#parameters) for what each variant means and
[Datasets](#datasets) for details on the datasets themselves.

| Variant | Size | {{range $places}} {{.Name}} | {{end}}
| --- | --- | {{range $places}} --- | {{end}}
{{range .Variants -}}
{{$variant := . -}}
{{$gpkg := printf "data/%s_%s.gpkg" $dataset.Name $variant.Name -}}
| [{{.Name}}]({{$gpkg}}) | {{filesize $gpkg}} | {{range $places -}}
{{$png := printf "data/%s_roundtrip_%s_%s.png" $dataset.Name $variant.Name .Name -}}
<a href="{{$png}}"><img src="{{$png}}" alt="{{$dataset.Name}} {{$variant.Name}} {{.Name}}"></a> |
{{- end}}
{{end}}

{{end}}