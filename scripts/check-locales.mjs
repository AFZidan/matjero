import { existsSync, readFileSync } from 'node:fs';
import { dirname, join } from 'node:path';
import { fileURLToPath } from 'node:url';

const app = process.argv[2];
if (!app) {
  throw new Error('app name is required');
}

const repoRoot = dirname(dirname(fileURLToPath(import.meta.url)));
const file = join(repoRoot, 'web', app, 'src', 'i18n', 'locales.ts');
if (!existsSync(file)) {
  throw new Error(`missing locale file: ${file}`);
}

const content = readFileSync(file, 'utf8');
for (const expected of ["'ar'", "'en'", 'rtl', 'ltr']) {
  if (!content.includes(expected)) {
    throw new Error(`${file} does not include ${expected}`);
  }
}

console.log(`${app}: locale foundation ok`);
