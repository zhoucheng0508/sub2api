/** @type {import('tailwindcss').Config} */
export default {
  content: ['./index.html', './src/**/*.{vue,js,ts,jsx,tsx}'],
  darkMode: 'class',
  theme: {
    extend: {
      colors: {
        // CUSTOM(VOTE-AI-THEME): warm public-site palette shared with the current shell.
        // 主色调 - 暖橙棕色系
        primary: {
          50: '#fef8f4',
          100: '#fdeee5',
          200: '#fbd8c5',
          300: '#f4b38f',
          400: '#df7b42',
          500: '#c45100',
          600: '#9c3f00',
          700: '#78350f',
          800: '#5d290f',
          900: '#3e1600',
          950: '#2a0d00'
        },
        // 辅助色 - 暖中性色
        accent: {
          50: '#fcf9f4',
          100: '#f6f3ee',
          200: '#eadfd8',
          300: '#d8c7bd',
          400: '#a58c7e',
          500: '#755f54',
          600: '#584238',
          700: '#453229',
          800: '#30231d',
          900: '#1c1c19',
          950: '#12110f'
        },
        // 覆盖默认灰阶，让亮色界面保持暖白基调
        gray: {
          50: '#fcf9f4',
          100: '#f6f3ee',
          200: '#eadfd8',
          300: '#d8c7bd',
          400: '#a58c7e',
          500: '#755f54',
          600: '#584238',
          700: '#453229',
          800: '#30231d',
          900: '#1c1c19',
          950: '#12110f'
        },
        // 深色模式背景 - 暖黑棕色系
        dark: {
          50: '#fcf9f4',
          100: '#f6f3ee',
          200: '#eadfd8',
          300: '#d8c7bd',
          400: '#a58c7e',
          500: '#755f54',
          600: '#584238',
          700: '#453229',
          800: '#30231d',
          900: '#211814',
          950: '#15100d'
        }
      },
      fontFamily: {
        sans: [
          'system-ui',
          '-apple-system',
          'BlinkMacSystemFont',
          'Segoe UI',
          'Roboto',
          'Helvetica Neue',
          'Arial',
          'PingFang SC',
          'Hiragino Sans GB',
          'Microsoft YaHei',
          'sans-serif'
        ],
        mono: ['ui-monospace', 'SFMono-Regular', 'Menlo', 'Monaco', 'Consolas', 'monospace']
      },
      boxShadow: {
        glass: '0 8px 32px rgba(0, 0, 0, 0.08)',
        'glass-sm': '0 4px 16px rgba(0, 0, 0, 0.06)',
        glow: '0 0 20px rgba(196, 81, 0, 0.18)',
        'glow-lg': '0 0 40px rgba(196, 81, 0, 0.24)',
        card: '0 1px 3px rgba(62, 22, 0, 0.035), 0 1px 2px rgba(62, 22, 0, 0.05)',
        'card-hover': '0 10px 36px rgba(62, 22, 0, 0.08)',
        'inner-glow': 'inset 0 1px 0 rgba(255, 255, 255, 0.1)'
      },
      backgroundImage: {
        'gradient-radial': 'radial-gradient(var(--tw-gradient-stops))',
        'gradient-primary': 'linear-gradient(135deg, #c45100 0%, #9c3f00 100%)',
        'gradient-dark': 'linear-gradient(135deg, #30231d 0%, #211814 100%)',
        'gradient-glass':
          'linear-gradient(135deg, rgba(255,255,255,0.1) 0%, rgba(255,255,255,0.05) 100%)',
        'mesh-gradient':
          'radial-gradient(at 40% 20%, rgba(196, 81, 0, 0.10) 0px, transparent 50%), radial-gradient(at 80% 0%, rgba(223, 123, 66, 0.07) 0px, transparent 50%), radial-gradient(at 0% 50%, rgba(156, 63, 0, 0.06) 0px, transparent 50%)'
      },
      animation: {
        'fade-in': 'fadeIn 0.3s ease-out',
        'slide-up': 'slideUp 0.3s ease-out',
        'slide-down': 'slideDown 0.3s ease-out',
        'slide-in-right': 'slideInRight 0.3s ease-out',
        'scale-in': 'scaleIn 0.2s ease-out',
        'pulse-slow': 'pulse 3s cubic-bezier(0.4, 0, 0.6, 1) infinite',
        shimmer: 'shimmer 2s linear infinite',
        glow: 'glow 2s ease-in-out infinite alternate'
      },
      keyframes: {
        fadeIn: {
          '0%': { opacity: '0' },
          '100%': { opacity: '1' }
        },
        slideUp: {
          '0%': { opacity: '0', transform: 'translateY(10px)' },
          '100%': { opacity: '1', transform: 'translateY(0)' }
        },
        slideDown: {
          '0%': { opacity: '0', transform: 'translateY(-10px)' },
          '100%': { opacity: '1', transform: 'translateY(0)' }
        },
        slideInRight: {
          '0%': { opacity: '0', transform: 'translateX(20px)' },
          '100%': { opacity: '1', transform: 'translateX(0)' }
        },
        scaleIn: {
          '0%': { opacity: '0', transform: 'scale(0.95)' },
          '100%': { opacity: '1', transform: 'scale(1)' }
        },
        shimmer: {
          '0%': { backgroundPosition: '-200% 0' },
          '100%': { backgroundPosition: '200% 0' }
        },
        glow: {
          '0%': { boxShadow: '0 0 20px rgba(196, 81, 0, 0.18)' },
          '100%': { boxShadow: '0 0 30px rgba(196, 81, 0, 0.30)' }
        }
      },
      backdropBlur: {
        xs: '2px'
      },
      borderRadius: {
        '4xl': '2rem'
      }
    }
  },
  plugins: []
}
