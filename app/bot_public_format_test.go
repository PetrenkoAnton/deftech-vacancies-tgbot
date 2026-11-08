package bot

import (
	"strings"
	"testing"

	"golang.org/x/net/html"
)

func TestFormatCompanyForPublic(t *testing.T) {
	tests := []struct {
		name       string
		company    string
		companyURL string
		want       string
	}{
		{name: "empty company", want: "-"},
		{name: "name without url", company: "Acme", want: "Acme"},
		{
			name:       "clickable markdown link",
			company:    "Acme",
			companyURL: "https://jobs.dou.ua/companies/acme/vacancies/",
			want:       "[Acme](https://jobs.dou.ua/companies/acme/vacancies/)",
		},
		{
			name:       "escapes brackets in company name",
			company:    "Acme [Labs]",
			companyURL: "https://jobs.dou.ua/companies/acme/vacancies/",
			want:       "[Acme (Labs)](https://jobs.dou.ua/companies/acme/vacancies/)",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := formatCompanyForPublic(tt.company, tt.companyURL)
			if got != tt.want {
				t.Fatalf("formatCompanyForPublic() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestNormalizeCompanyURL(t *testing.T) {
	tests := []struct {
		raw  string
		want string
	}{
		{raw: "https://jobs.dou.ua/companies/acme/vacancies/", want: "https://jobs.dou.ua/companies/acme/vacancies/"},
		{raw: "http://jobs.dou.ua/companies/acme/vacancies/", want: "http://jobs.dou.ua/companies/acme/vacancies/"},
		{raw: "//jobs.dou.ua/companies/acme/vacancies/", want: "https://jobs.dou.ua/companies/acme/vacancies/"},
		{raw: "/companies/acme/vacancies/", want: "https://jobs.dou.ua/companies/acme/vacancies/"},
		{raw: "jobs.dou.ua/companies/acme/vacancies/", want: "https://jobs.dou.ua/companies/acme/vacancies/"},
		{raw: "  ", want: ""},
	}

	for _, tt := range tests {
		got := normalizeCompanyURL(tt.raw)
		if got != tt.want {
			t.Fatalf("normalizeCompanyURL(%q) = %q, want %q", tt.raw, got, tt.want)
		}
	}
}

func TestFormatPublicVacanciesMessageMakesCompanyClickable(t *testing.T) {
	b := &Bot{}
	got := b.formatPublicVacanciesMessage([]VacancyInfo{{
		Title:      "Go Developer",
		URL:        "https://jobs.dou.ua/companies/acme/vacancies/123/",
		Company:    "Acme",
		CompanyURL: "https://jobs.dou.ua/companies/acme/vacancies/",
	}})

	wantCompany := "[Acme](https://jobs.dou.ua/companies/acme/vacancies/)"
	if !strings.Contains(got, wantCompany) {
		t.Fatalf("public message missing clickable company %q\n%s", wantCompany, got)
	}
	if !strings.HasPrefix(got, "**New vacancy:**") {
		t.Fatalf("single vacancy heading = %q, want **New vacancy:**", got)
	}
}

func TestNewVacanciesHeading(t *testing.T) {
	if got := newVacanciesHeading(1); got != "**New vacancy:**\n\n" {
		t.Fatalf("newVacanciesHeading(1) = %q", got)
	}
	if got := newVacanciesHeading(2); got != "**New vacancies:**\n\n" {
		t.Fatalf("newVacanciesHeading(2) = %q", got)
	}

	b := &Bot{}
	plural := b.formatPublicVacanciesMessage([]VacancyInfo{
		{Title: "Go Developer", URL: "https://example.com/1", Company: "Acme"},
		{Title: "Rust Developer", URL: "https://example.com/2", Company: "Acme"},
	})
	if !strings.HasPrefix(plural, "**New vacancies:**") {
		t.Fatalf("plural heading = %q, want **New vacancies:**", plural)
	}
}

func TestFindJobTitlesDeftechCapturesCompanyURL(t *testing.T) {
	htmlSrc := `<html><body>
		<a href="https://jobs.dou.ua/jobs/companies/acme/vacancies/123/">Go Developer</a>
		<a href="https://jobs.dou.ua/companies/acme/vacancies/">Acme</a>
	</body></html>`
	doc, err := html.Parse(strings.NewReader(htmlSrc))
	if err != nil {
		t.Fatalf("parse html: %v", err)
	}

	got := findJobTitlesDeftech(doc)
	if len(got) != 1 {
		t.Fatalf("got %d vacancies, want 1", len(got))
	}
	if got[0].Company != "Acme" {
		t.Fatalf("company = %q, want Acme", got[0].Company)
	}
	wantURL := "https://jobs.dou.ua/companies/acme/vacancies/"
	if got[0].CompanyURL != wantURL {
		t.Fatalf("company URL = %q, want %q", got[0].CompanyURL, wantURL)
	}
}
