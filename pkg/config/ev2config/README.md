# Ev2 Central Configuration

## Accessing Central Configuration

It is challenging to automate the download of central configuration files from Ev2. While the `ev2` CLI does work for
public cloud values, an escort and SAW would be required to use it for sovereign clouds. Use the [portal](https://ev2portal.azure.net/#config/)
to access the values instead and populate `public.config.json` and `ff.config.json` before sanitizing them. 