using MudBlazor;

namespace Summon;

public static class AegisTheme
{
    public static readonly MudTheme Dark = new()
    {
        PaletteDark = new PaletteDark
        {
            Black = "#010409",
            White = "#e6edf3",

            Primary = "#238636",
            PrimaryContrastText = "#ffffff",
            Secondary = "#1f6feb",
            SecondaryContrastText = "#ffffff",
            Tertiary = "#8957e5",
            TertiaryContrastText = "#ffffff",

            Info = "#58a6ff",
            Success = "#2ea043",
            Warning = "#d29922",
            Error = "#f85149",
            Dark = "#0d1117",

            TextPrimary = "#e6edf3",
            TextSecondary = "#7d8590",
            TextDisabled = "#484f58",

            ActionDefault = "#e6edf3",
            ActionDisabled = "rgba(125,133,144,0.3)",
            ActionDisabledBackground = "rgba(125,133,144,0.12)",

            Background = "#0d1117",
            BackgroundGray = "#161b22",
            Surface = "#161b22",

            DrawerBackground = "#010409",
            DrawerText = "#e6edf3",
            DrawerIcon = "#7d8590",
            AppbarBackground = "#010409",
            AppbarText = "#e6edf3",

            LinesDefault = "#30363d",
            LinesInputs = "#30363d",
            TableLines = "#30363d",
            TableStriped = "rgba(48,54,61,0.3)",
            TableHover = "rgba(48,54,61,0.4)",
            Divider = "#30363d",
            DividerLight = "rgba(48,54,61,0.6)",

            HoverOpacity = 0.08,
        }
    };
}
