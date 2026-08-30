export const locales = ['ar', 'en'] as const;
export type Locale = (typeof locales)[number];

export const messages: Record<Locale, { appName: string; status: string }> = {
  ar: {
    appName: 'لوحة المورد',
    status: 'جاهز للبناء'
  },
  en: {
    appName: 'Supplier Dashboard',
    status: 'Ready for build'
  }
};

export function directionFor(locale: Locale): 'rtl' | 'ltr' {
  return locale === 'ar' ? 'rtl' : 'ltr';
}
