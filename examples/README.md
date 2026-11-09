# Examples

This folder contains some examples of recordings and exports.
It's intended to be used as a demonstration of features and also to track the file size.

| Example                                    | Description                                                                              |
| ------------------------------------------ | ---------------------------------------------------------------------------------------- |
| [256colors](256colors.svg)                 | script to print the 256 xterm colors as background and foreground                        |
| [htop](htop.svg)                           | running htop to see progress bars, headers and pagination                                |
| [session](session.svg)                     | simple terminal session running various commands like ls and cat                         |
| [444816](444816.svg)                       | Asciicast recording of an equilibrium pendulum to see compatibility with asciinema files |
| [444816 borderless](444816_borderless.svg) | Same as 444816 but without a terminal window                                             |
| [rgb](rgb.svg)                             | Sample of rgb output                                                                     |

> Files are not embedded here to alleviate loading

### GIF and WebM

The same recordings exported as raster images with `termsvg export -f gif` and `termsvg export -f webm`
(WebM needs `ffmpeg` with `libvpx-vp9`).

| Example                  | GIF                | WebM                 |
| ------------------------ | ------------------ | -------------------- |
| [htop](htop.svg)         | [htop.gif](htop.gif)       | [htop.webm](htop.webm)       |
| [rgb](rgb.svg)           | [rgb.gif](rgb.gif)         | [rgb.webm](rgb.webm)         |
| [session](session.svg)   | [session.gif](session.gif) | [session.webm](session.webm) |

`256colors` and `444816` are not exported to these formats to keep the repository small
(their GIFs are about 0.9 MB and 4 MB).

## Sizes

This table tracks size changes between the first release of the example and the last one as examples are updated on each code change to reflect new features and optimizations.

<!--SIZES_START-->
| File | Iterations | First Size | Current Size | Variation |
|------|:----------:|------------|--------------|-----------|
| 256colors.svg | 7 | 954.73kB | 39.20kB | 95.8942% |
| 444816.svg | 10 | 3.42MB | 142.10kB | 95.8383% |
| 444816_borderless.svg | 5 | 3.11MB | 141.96kB | 95.4290% |
| htop.svg | 8 | 74.15kB | 15.27kB | 79.4066% |
| rgb.svg | 5 | 95.53kB | 6.75kB | 92.9349% |
| session.svg | 9 | 462.64kB | 18.51kB | 96.0003% |
| htop.gif | 1 | 158.63kB | 158.63kB | 0.0000% |
| rgb.gif | 1 | 136.76kB | 136.76kB | 0.0000% |
| session.gif | 1 | 480.20kB | 480.20kB | 0.0000% |
| htop.webm | 1 | 367.87kB | 367.87kB | 0.0000% |
| rgb.webm | 1 | 20.28kB | 20.28kB | 0.0000% |
| session.webm | 1 | 753.98kB | 753.98kB | 0.0000% |

<!--SIZES_END-->

> Table generated using [update-filesize.sh](/scripts/update-filesize.sh) script. Regenerate the examples
> and the table with `task examples` (or `task examples:svg`, `examples:gif`, `examples:webm` individually).
> GIF output is fully reproducible; WebM output is reproducible for a given ffmpeg/libvpx version.
