import type { Address, Hex } from "viem";
import {
  expectObject,
  expectOnlyKeys,
  optionalField,
  parseAddress,
  parseArray,
  parseBoolean,
  parseEnum,
  parseNonEmptyString,
  parseSafeInteger,
  parseString,
  parseUnsignedDecimal,
  requiredField,
  type JsonObject,
  type ValueParser,
} from "../command-harness.js";

export function parseEmptyInput(
  value: unknown,
  label: string
): Record<string, never> {
  const input = expectObject(value, label);
  expectOnlyKeys(input, [], label);
  return {};
}

export function parseInputObject(
  value: unknown,
  label: string,
  keys: readonly string[]
): JsonObject {
  const input = expectObject(value, label);
  expectOnlyKeys(input, keys, label);
  return input;
}

export function stringField(
  input: JsonObject,
  key: string,
  label = "input"
): string {
  return requiredField(input, key, parseNonEmptyString, label);
}

export function optionalStringField(
  input: JsonObject,
  key: string,
  label = "input"
): string | undefined {
  return optionalField(input, key, parseString, label);
}

export function addressField(
  input: JsonObject,
  key: string,
  label = "input"
): Address {
  return requiredField(input, key, parseAddress, label);
}

export function optionalAddressField(
  input: JsonObject,
  key: string,
  label = "input"
): Address | undefined {
  return optionalField(input, key, parseAddress, label);
}

export function addressArrayField(
  input: JsonObject,
  key: string,
  label = "input"
): Address[] {
  return requiredField(
    input,
    key,
    (value, fieldLabel) =>
      parseArray(value, fieldLabel, parseAddress, { minLength: 1 }),
    label
  );
}

export function bigintField(
  input: JsonObject,
  key: string,
  label = "input"
): bigint {
  return BigInt(requiredField(input, key, parseUnsignedDecimal, label));
}

export function optionalBigintField(
  input: JsonObject,
  key: string,
  label = "input"
): bigint | undefined {
  const value = optionalField(input, key, parseUnsignedDecimal, label);
  return value === undefined ? undefined : BigInt(value);
}

export function uint32Field(
  input: JsonObject,
  key: string,
  label = "input"
): number {
  const value = requiredField(input, key, parseSafeInteger, label);
  if (value > 0xffff_ffff) {
    throw new Error(`${label}.${key} exceeds uint32`);
  }
  return value;
}

export function optionalUint32Field(
  input: JsonObject,
  key: string,
  label = "input"
): number | undefined {
  const value = optionalField(input, key, parseSafeInteger, label);
  if (value !== undefined && value > 0xffff_ffff) {
    throw new Error(`${label}.${key} exceeds uint32`);
  }
  return value;
}

export function booleanField(
  input: JsonObject,
  key: string,
  label = "input"
): boolean {
  return requiredField(input, key, parseBoolean, label);
}

export function optionalBooleanField(
  input: JsonObject,
  key: string,
  label = "input"
): boolean | undefined {
  return optionalField(input, key, parseBoolean, label);
}

export function hexField(input: JsonObject, key: string, label = "input"): Hex {
  return requiredField(input, key, parseHex, label);
}

export function optionalHexField(
  input: JsonObject,
  key: string,
  label = "input"
): Hex | undefined {
  return optionalField(input, key, parseHex, label);
}

export function enumField<const T extends readonly string[]>(
  input: JsonObject,
  key: string,
  choices: T,
  label = "input"
): T[number] {
  return requiredField(
    input,
    key,
    (value, fieldLabel) => parseEnum(value, fieldLabel, choices),
    label
  );
}

export function optionalParsedField<T>(
  input: JsonObject,
  key: string,
  parser: ValueParser<T>,
  label = "input"
): T | undefined {
  return optionalField(input, key, parser, label);
}

function parseHex(value: unknown, label: string): Hex {
  const parsed = parseString(value, label);
  if (!/^0x(?:[0-9a-fA-F]{2})*$/.test(parsed)) {
    throw new Error(`${label} must be 0x-prefixed hex bytes`);
  }
  return parsed as Hex;
}
