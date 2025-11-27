# DESIGN.md

This file defines the visual design system including colors, typography, spacing, and component specifications.
The tables below define the design tokens and their values, distinguishing between light and dark mode where required.

## Theme

### Colors

CSS variable prefix: `--color-`.

| Token                        | Light                       | Dark                        |
|------------------------------|-----------------------------|-----------------------------|
| $color-01dp                  | white                       | #1E2A38                     |
| $color-02dp                  | white                       | #23323F                     |
| $color-03dp                  | white                       | #283A47                     |
| $color-bg-body               | #F2F2F3                     | #192734                     |
| $color-button-border         | hsl(208, 9.0%, 73.0%)       | hsl(208, 11.0%, 45.0%)      |
| $color-button-hover-bg       | rgba(255, 255, 255, 0.15)   | rgba(255, 255, 255, 0.15)   |
| $color-button-primary-text   | white                       | hsl(209, 88.0%, 7.0%)       |
| $color-button-text           | hsla(0, 0%, 0%, 0.54)       | hsla(0, 0%, 100%, 0.74)     |
| $color-danger                | hsl(347, 100%, 60.0%)       | hsl(347, 100%, 60.0%)       |
| $color-hiContrast            | black                       | white                       |
| $color-input-bg              | white                       | #14202A                     |
| $color-input-border          | #B4BBC1                     | #67737F                     |
| $color-logo-text             | #04192D                     | white                       |
| $color-primary               | hsl(68, 53.0%, 36.0%)       | hsl(68, 79.0%, 60.0%)       |
| $color-primary-hover         | hsl(68, 53.0%, 42.0%)       | hsl(68, 79.0%, 66.0%)       |
| $color-text                  | hsla(0, 0%, 0%, 0.74)       | hsla(0, 0%, 100%, 0.74)     |
| $color-text-subtle           | hsla(0, 0%, 0%, 0.51)       | hsla(0, 0%, 100%, 0.51)     |

### Font sizes

CSS variables prefix: `--font-size-`.

| Token | Value |
|-------|-------|
| $0    | 11px  |
| $1    | 12px  |
| $2    | 13px  |
| $3    | 14px  |
| $4    | 16px  |
| $5    | 18px  |
| $6    | 20px  |
| $7    | 22px  |
| $8    | 24px  |
| $9    | 26px  |
| $10   | 28px  |
| $11   | 32px  |
| $12   | 38px  |

### Heights, widths, and line heights

CSS variable prefix: `--height-`.

| Token | Value |
|-------|-------|
| $1    | 4px   |
| $2    | 8px   |
| $3    | 16px  |
| $4    | 20px  |
| $5    | 24px  |
| $6    | 32px  |
| $7    | 40px  |

### Margins, paddings

CSS variable prefix: `--spacing-`.

| Token | Value |
|-------|-------|
| $1    | 4px   |
| $2    | 8px   |
| $3    | 16px  |
| $4    | 20px  |
| $5    | 24px  |
| $6    | 32px  |
| $7    | 48px  |

### Radii

CSS variable prefix: `--radius-`.

| Token | Value |
|-------|-------|
| $1    | 4px   |
| $2    | 6px   |
| $3    | 8px   |
| $4    | 12px  |

### Typography

| Property      | Value                |
|---------------|----------------------|
| Font family   | Rubik (Google Fonts) |
| Default weight| 400 (Regular)        |

**Heading (h1):**

- Font size: $10
- Font weight: 400
- Line height: 1.33

## Components

### Button

**Default:**

- Display: inline-flex
- Align items: center
- Justify content: center
- Gap: $1
- Background color: transparent
- Text color: hsla(0, 0%, 0%, 0.54) (light), hsla(0, 0%, 100%, 0.74) (dark)
- Border: 2px solid hsl(208, 9.0%, 73.0%) (light), 2px solid hsl(208, 11.0%, 45.0%) (dark)
- Border radius: $3
- Height: $6
- Padding: 0 $3
- Font size: $3
- Line height: 1

**Hover:**

- Background color: rgba(255, 255, 255, 0.15)

**Primary (additional class):**

- Background color: $primary
- Text color: $button-primary-text
- Border: none

**Primary hover:**

- Background color: $primary-hover

**Sizes:**

| Size    | Height | Font Size | Line Height |
|---------|--------|-----------|-------------|
| small   | $5     | $1        | $5          |
| default | $6     | $3        | $6          |
| large   | $7     | $3        | $7          |

### Input

**Label:**

- Font size: $0
- Margin bottom: 5px

**Default:**

- Width: 100%
- Height: $6
- Padding: 0 $3
- Font size: $3
- Line height: $6
- Border: 1px solid $input-border
- Border radius: $3
- Color: $text
- Background: $input-bg
- Transition: border-color 0.1s ease-in-out

**Focus:**

- Border color: $primary
- Transition: border-color 0.1s ease-in-out

**Sizes:**

| Size    | Height | Font Size | Line Height | Padding |
|---------|--------|-----------|-------------|---------|
| small   | $5     | $1        | $5          | 0 $2    |
| default | $6     | $3        | $6          | 0 $3    |
| large   | $7     | $3        | $7          | 0 $3    |

### Text Styles

**Subtitle text:**

- Font size: $2
- Text align: center
- Margin bottom: $6

**Subtitle variants:**

- `.error`: Color $danger
- `.info`: Color $text-subtle

### Link

**Default:**

- Color: inherit
- Text decoration: none

**Hover:**

- Color: $text
- Text decoration: underline
- Text decoration color: $text-subtle

### Card

A container component that wraps other components with elevation and depth.

**Default:**

- Display: block
- Position: relative
- Padding: $3
- Border radius: $3
- Border: none

**Elevations:**

| Elevation | Background Color | Box Shadow                                                                                                    |
|-----------|------------------|---------------------------------------------------------------------------------------------------------------|
| 1         | $01dp            | rgba(0, 0, 0, 0.2) 0px 1px 5px 0px, rgba(0, 0, 0, 0.12) 0px 3px 1px -2px, rgba(0, 0, 0, 0.14) 0px 2px 2px 0px |
| 2         | $02dp            | rgba(0, 0, 0, 0.2) 0px 2px 4px -1px, rgba(0, 0, 0, 0.12) 0px 1px 10px 0px, rgba(0, 0, 0, 0.14) 0px 4px 5px 0px |
| 3         | $03dp            | rgba(0, 0, 0, 0.2) 0px 3px 5px -1px, rgba(0, 0, 0, 0.12) 0px 1px 18px 0px, rgba(0, 0, 0, 0.14) 0px 6px 10px 0px |

### Form

**Form group:**

- Margin bottom: $4

**Form group (last):**

- Margin bottom: $7

### Tabs

A navigation component that allows users to switch between different views.

**Tabs container:**

- Display: flex
- Each tab button takes equal width (flex: 1)

**Tab button (default):**

- Background: transparent
- Color: $text-subtle
- Border: none
- Border bottom: 1px solid $border
- Padding: $3
- Font size: $4
- Font weight: 600
- Text align: center
- Cursor: pointer

**Tab button (hover):**

- Color: $hiContrast

**Tab button (active):**

- Color: $primary
- Border bottom: 2px solid $primary
