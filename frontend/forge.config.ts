import type { ForgeConfig } from "@electron-forge/shared-types";
import { MakerSquirrel } from "@electron-forge/maker-squirrel";
import { MakerZIP } from "@electron-forge/maker-zip";
import { MakerDMG } from "@electron-forge/maker-dmg";
import { MakerDeb } from "@electron-forge/maker-deb";
import { MakerRpm } from "@electron-forge/maker-rpm";
import { VitePlugin } from "@electron-forge/plugin-vite";
import { FusesPlugin } from "@electron-forge/plugin-fuses";
import { FuseV1Options, FuseVersion } from "@electron/fuses";
import { macSigning, updateResources } from "./scripts/mac-signing";

const config: ForgeConfig = {
  packagerConfig: {
    asar: true,
    name: "Babysitter",
    executableName: "babysitter",
    appBundleId: "com.deividfortuna.babysitter",
    icon: "assets/icon",
    ...macSigning(process.env),
    extraResource: [
      "daemon",
      "assets/icon.png",
      "assets/trayTemplate.png",
      "assets/trayTemplate@2x.png",
      ...updateResources(process.env),
    ],
  },
  rebuildConfig: {},
  makers: [
    // The installer of Windows. It installs per user, under
    // %LocalAppData%\babysitter, and makes the shortcuts. No MSI: it needs
    // the WiX toolkit, and nothing deploys the app by policy.
    new MakerSquirrel({ setupIcon: "assets/icon.ico", setupExe: "Babysitter-Setup.exe", noMsi: true }),
    new MakerZIP({}, ["darwin"]),
    new MakerDMG({ name: "Babysitter", icon: "assets/icon.icns", format: "ULFO" }, ["darwin"]),
    new MakerRpm({ options: { icon: "assets/icon.png" } }),
    new MakerDeb({ options: { icon: "assets/icon.png" } }),
  ],
  plugins: [
    new VitePlugin({
      build: [
        { entry: "src/main.ts", config: "vite.main.config.mts", target: "main" },
        { entry: "src/preload.ts", config: "vite.preload.config.mts", target: "preload" },
      ],
      renderer: [{ name: "main_window", config: "vite.renderer.config.mts" }],
    }),
    new FusesPlugin({
      version: FuseVersion.V1,
      [FuseV1Options.RunAsNode]: false,
      [FuseV1Options.EnableCookieEncryption]: true,
      [FuseV1Options.EnableNodeOptionsEnvironmentVariable]: false,
      [FuseV1Options.EnableNodeCliInspectArguments]: false,
      [FuseV1Options.EnableEmbeddedAsarIntegrityValidation]: true,
      [FuseV1Options.OnlyLoadAppFromAsar]: true,
    }),
  ],
};

export default config;
