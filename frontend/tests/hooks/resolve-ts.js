// Source files import each other the way Vite expects ("./range"), which Node's
// resolver will not follow. This hook adds the extension back so the test runner
// loads exactly the modules the app bundles, with no build step in between.
export async function resolve(specifier, context, next) {
  if (specifier.startsWith('.') && !/\.[a-z]+$/i.test(specifier)) {
    try {
      return await next(`${specifier}.ts`, context)
    } catch {
      // not a TypeScript module, fall through to the normal rules
    }
  }
  return next(specifier, context)
}
