// Help for Profile (design §7.8). Facts: internal/api/auth_handlers.go:125-129, 137-143, 164, 174-195;
// internal/api/validate.go:25-38.
export const profile = {
  summary: {
    name: 'Profile',
    text: 'shows your account and sets the email and phone number your alerts go to. You can change your password here.',
  },
  items: {
    role: {
      title: 'Role',
      text: ['What you can do: viewers look; operators also manage targets; admins also manage users and settings.'],
    },
    email: {
      title: 'Email',
      text: ['Where email alerts go.'],
    },
    phone: {
      title: 'Phone',
      text: ['Where Signal alerts go: an optional +, then 7 to 15 digits, no spaces.'],
    },
    save: {
      title: 'Save',
      text: ["Saves email and phone. Emptying a field clears it."],
    },
    pw: {
      title: 'New Password',
      text: ['15 to 32 characters. Letters such as ş or ğ count as two.'],
    },
    change: {
      title: 'Change Password',
      text: ['Signs you out everywhere else.'],
    },
  },
}
