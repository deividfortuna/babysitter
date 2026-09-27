import ghosttyWasmDataUrl from "./vendor/ghostty-vt.wasm?inline";

type WasmFunction = (...args: Array<number | bigint>) => number;

interface TypeField {
  readonly offset: number;
  readonly size: number;
  readonly type: string;
}

interface TypeLayout {
  readonly size: number;
  readonly align: number;
  readonly fields: Readonly<Record<string, TypeField>>;
}

type TypeLayouts = Readonly<Record<string, TypeLayout>>;

const textDecoder = new TextDecoder();

function decodeDataUrl(dataUrl: string): Uint8Array<ArrayBuffer> {
  const binary = atob(dataUrl.slice(dataUrl.indexOf(",") + 1));
  return Uint8Array.from(binary, (char) => char.charCodeAt(0));
}

export class GhosttyRuntime {
  readonly memory: WebAssembly.Memory;
  readonly layouts: TypeLayouts;
  private readonly exports: WebAssembly.Exports;
  private memoryView: DataView;

  private constructor(instance: WebAssembly.Instance) {
    this.exports = instance.exports;
    const memory = instance.exports.memory;
    if (!(memory instanceof WebAssembly.Memory)) {
      throw new Error("libghostty-vt did not export WebAssembly memory");
    }
    this.memory = memory;
    this.memoryView = new DataView(memory.buffer);
    const jsonPointer = this.call("ghostty_type_json");
    const bytes = new Uint8Array(memory.buffer);
    let end = jsonPointer;
    while (end < bytes.length && bytes[end] !== 0) end += 1;
    this.layouts = JSON.parse(textDecoder.decode(bytes.subarray(jsonPointer, end))) as TypeLayouts;
  }

  static async load(): Promise<GhosttyRuntime> {
    let instance: WebAssembly.Instance | undefined;
    const imports = {
      env: {
        log: (pointer: number, length: number) => {
          const memory = instance?.exports.memory;
          if (!(memory instanceof WebAssembly.Memory)) return;
          console.debug("[libghostty-vt]", textDecoder.decode(new Uint8Array(memory.buffer, pointer, length)));
        },
      },
    };
    const result = await WebAssembly.instantiate(decodeDataUrl(ghosttyWasmDataUrl), imports);
    instance = result.instance;
    return new GhosttyRuntime(result.instance);
  }

  call(name: string, ...args: Array<number | bigint>): number {
    const fn = this.exports[name];
    if (typeof fn !== "function") {
      throw new Error(`libghostty-vt export is unavailable: ${name}`);
    }
    return (fn as WasmFunction)(...args);
  }

  layout(name: string): TypeLayout {
    const layout = this.layouts[name];
    if (!layout) throw new Error(`libghostty-vt type layout is unavailable: ${name}`);
    return layout;
  }

  alloc(size: number): number {
    const pointer = this.call("ghostty_wasm_alloc_u8_array", size);
    if (pointer === 0) throw new Error(`libghostty-vt failed to allocate ${size} bytes`);
    new Uint8Array(this.memory.buffer, pointer, size).fill(0);
    return pointer;
  }

  free(pointer: number, size: number): void {
    if (pointer !== 0) this.call("ghostty_wasm_free_u8_array", pointer, size);
  }

  allocOpaque(): number {
    const pointer = this.call("ghostty_wasm_alloc_opaque");
    if (pointer === 0) throw new Error("libghostty-vt failed to allocate an opaque pointer");
    new DataView(this.memory.buffer).setUint32(pointer, 0, true);
    return pointer;
  }

  freeOpaque(pointer: number): void {
    if (pointer !== 0) this.call("ghostty_wasm_free_opaque", pointer);
  }

  readPointer(slot: number): number {
    return this.currentMemoryView().getUint32(slot, true);
  }

  view(pointer: number, size?: number): DataView {
    return new DataView(this.memory.buffer, pointer, size);
  }

  bytes(pointer: number, size: number): Uint8Array {
    return new Uint8Array(this.memory.buffer, pointer, size);
  }

  setField(pointer: number, structName: string, fieldName: string, value: number): void {
    const field = this.field(structName, fieldName);
    const view = this.currentMemoryView();
    const offset = pointer + field.offset;
    switch (field.type) {
      case "bool":
      case "u8":
        view.setUint8(offset, value);
        return;
      case "u16":
        view.setUint16(offset, value, true);
        return;
      case "i32":
        view.setInt32(offset, value, true);
        return;
      case "u32":
      case "enum":
        view.setUint32(offset, value, true);
        return;
      case "u64":
        view.setBigUint64(offset, BigInt(value), true);
        return;
      default:
        throw new Error(`Unsupported libghostty-vt field type: ${field.type}`);
    }
  }

  readField(pointer: number, structName: string, fieldName: string): number {
    const field = this.field(structName, fieldName);
    const view = this.currentMemoryView();
    const offset = pointer + field.offset;
    switch (field.type) {
      case "bool":
      case "u8":
        return view.getUint8(offset);
      case "u16":
        return view.getUint16(offset, true);
      case "i32":
        return view.getInt32(offset, true);
      case "u32":
      case "enum":
        return view.getUint32(offset, true);
      case "u64":
        return Number(view.getBigUint64(offset, true));
      default:
        throw new Error(`Unsupported libghostty-vt field type: ${field.type}`);
    }
  }

  private field(structName: string, fieldName: string): TypeField {
    const field = this.layout(structName).fields[fieldName];
    if (!field) throw new Error(`libghostty-vt field is unavailable: ${structName}.${fieldName}`);
    return field;
  }

  private currentMemoryView(): DataView {
    if (this.memoryView.buffer !== this.memory.buffer) {
      this.memoryView = new DataView(this.memory.buffer);
    }
    return this.memoryView;
  }
}

let runtimePromise: Promise<GhosttyRuntime> | null = null;

export function loadGhosttyRuntime(): Promise<GhosttyRuntime> {
  runtimePromise ??= GhosttyRuntime.load().catch((error: unknown) => {
    runtimePromise = null;
    throw error;
  });
  return runtimePromise;
}
