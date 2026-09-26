export type SettingsGate = {
  isSuccess: boolean;
  isError: boolean;
};

export function presents(supported: boolean | null, settings: SettingsGate): boolean | null {
  if (supported === null) return null;
  if (!supported) return false;
  if (settings.isSuccess) return true;
  if (settings.isError) return false;
  return null;
}
