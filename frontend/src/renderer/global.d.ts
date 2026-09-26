import type { BabysitterBridge } from "../preload";

declare global {
  interface Window {
    babysitter?: BabysitterBridge;
  }
}

export {};
