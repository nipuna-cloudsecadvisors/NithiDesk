package courts

// Default reference data seeded into a new firm.
//
// [NEEDS LAWYER CONFIRMATION] Every list here is a starting point for the
// pilot firm to correct (doc 05 §2–3, doc 20 §3): court names and Sinhala
// terms, proceeding types, stage and purpose catalogues, and especially the
// authority assigned to each date source (doc 06 §3, logical model Q4). They
// are data, editable by an administrator, never logic.

// SeedItem is one reference row.
type SeedItem struct {
	Code       string
	NameEN     string
	NameSI     string
	Sort       int
	Attributes map[string]any
}

// SeedCourt is one court.
type SeedCourt struct {
	Code       string
	Level      string
	NameEN     string
	NameSI     string
	LocationEN string
	LocationSI string
	Patterns   []CaseNumberPattern
}

// Seed is the default catalogue, keyed by reference kind.
var Seed = map[string][]SeedItem{
	KindCourtLevel: {
		{"sc", "Supreme Court", "ශ්\u200Dරේෂ්ඨාධිකරණය", 10, nil},
		{"ca", "Court of Appeal", "අභියාචනාධිකරණය", 20, nil},
		{"hcca", "Civil Appellate High Court", "සිවිල් අභියාචන මහාධිකරණය", 30, nil},
		{"chc", "Commercial High Court", "වාණිජ මහාධිකරණය", 35, nil},
		{"hc", "High Court", "මහාධිකරණය", 40, nil},
		{"dc", "District Court", "දිසා අධිකරණය", 50, nil},
		{"mc", "Magistrate's Court", "මහේස්ත්\u200Dරාත් අධිකරණය", 60, nil},
		{"pc", "Primary Court", "ප්\u200Dරාථමික අධිකරණය", 65, nil},
		{"lt", "Labour Tribunal", "කම්කරු විනිශ්චය සභාව", 70, nil},
	},
	KindPracticeArea: {
		{"land", "Land and property", "ඉඩම් හා දේපළ", 10, nil},
		{"partition", "Partition", "බෙදුම්", 15, nil},
		{"money_recovery", "Money recovery", "මුදල් අයකර ගැනීම", 20, nil},
		{"commercial", "Commercial", "වාණිජ", 30, nil},
		{"family", "Family and divorce", "පවුල් හා දික්කසාද", 40, nil},
		{"testamentary", "Testamentary", "අන්තිම කැමති පත්\u200Dර", 50, nil},
		{"criminal", "Criminal", "අපරාධ", 60, nil},
		{"labour", "Labour", "කම්කරු", 70, nil},
		{"fundamental_rights", "Fundamental rights", "මූලික අයිතිවාසිකම්", 80, nil},
		{"writs", "Writs", "රිට්", 90, nil},
		{"other", "Other", "වෙනත්", 999, nil},
	},
	KindProceedingType: {
		{"dc_land", "Land action (DC)", "ඉඩම් නඩුව (දිසා)", 10, attrs("dc", "civil")},
		{"dc_partition", "Partition action (DC)", "බෙදුම් නඩුව (දිසා)", 11, attrs("dc", "civil")},
		{"dc_money", "Money recovery (DC)", "මුදල් අයකර ගැනීමේ නඩුව (දිසා)", 12, attrs("dc", "civil")},
		{"dc_divorce", "Divorce (DC)", "දික්කසාද නඩුව (දිසා)", 13, attrs("dc", "civil")},
		{"dc_testamentary", "Testamentary (DC)", "අන්තිම කැමති පත්\u200Dර නඩුව (දිසා)", 14, attrs("dc", "testamentary")},
		{"dc_other", "Other civil action (DC)", "වෙනත් සිවිල් නඩුව (දිසා)", 19, attrs("dc", "civil")},
		{"mc_b_report", "B report / investigation (MC)", "බී වාර්තාව (මහේස්ත්\u200Dරාත්)", 20, attrs("mc", "criminal")},
		{"mc_summary_trial", "Summary trial (MC)", "සාරාංශ නඩු විභාගය (මහේස්ත්\u200Dරාත්)", 21, attrs("mc", "criminal")},
		{"mc_maintenance", "Maintenance (MC)", "නඩත්තු නඩුව (මහේස්ත්\u200Dරාත්)", 22, attrs("mc", "civil")},
		{"pc_s66", "Primary Court s.66 dispute", "ප්\u200Dරාථමික අධිකරණ 66 වගන්තිය", 25, attrs("pc", "civil")},
		{"hc_indictment", "Indictment (HC)", "අධිචෝදනා පත්\u200Dරය (මහාධිකරණ)", 30, attrs("hc", "criminal")},
		{"hc_revision", "Revision / appeal from MC (HC)", "ප්\u200Dරතිශෝධන (මහාධිකරණ)", 31, attrs("hc", "appellate")},
		{"chc_civil", "Commercial action (CHC)", "වාණිජ නඩුව", 35, attrs("chc", "civil")},
		{"hcca_appeal", "Final appeal (HCCA)", "අවසාන අභියාචනය (සිවිල් අභියාචන)", 40, attrs("hcca", "appellate")},
		{"hcca_leave", "Leave to appeal (HCCA)", "අභියාචනා කිරීමට අවසර (සිවිල් අභියාචන)", 41, attrs("hcca", "appellate")},
		{"ca_appeal", "Appeal (CA)", "අභියාචනය (අභියාචනාධිකරණ)", 50, attrs("ca", "appellate")},
		{"ca_writ", "Writ application (CA)", "රිට් ඉල්ලීම (අභියාචනාධිකරණ)", 51, attrs("ca", "writ")},
		{"ca_revision", "Revision application (CA)", "ප්\u200Dරතිශෝධන ඉල්ලීම (අභියාචනාධිකරණ)", 52, attrs("ca", "appellate")},
		{"sc_leave", "Special leave to appeal (SC)", "විශේෂ අභියාචනා අවසර (ශ්\u200Dරේෂ්ඨ)", 60, attrs("sc", "appellate")},
		{"sc_appeal", "Appeal (SC)", "අභියාචනය (ශ්\u200Dරේෂ්ඨ)", 61, attrs("sc", "appellate")},
		{"sc_fr", "Fundamental rights (SC)", "මූලික අයිතිවාසිකම් (ශ්\u200Dරේෂ්ඨ)", 62, attrs("sc", "fr")},
		{"lt_application", "Labour Tribunal application", "කම්කරු විනිශ්චය සභා ඉල්ලීම", 70, attrs("lt", "labour")},
	},
	KindPartyRoleType: {
		{"plaintiff", "Plaintiff", "පැමිණිලිකරු", 10, side("applicant_side")},
		{"defendant", "Defendant", "විත්තිකරු", 20, side("respondent_side")},
		{"added_defendant", "Added defendant", "එකතු කළ විත්තිකරු", 25, side("respondent_side")},
		{"petitioner", "Petitioner", "පෙත්සම්කරු", 30, side("applicant_side")},
		{"respondent", "Respondent", "වගඋත්තරකරු", 40, side("respondent_side")},
		{"appellant", "Appellant", "අභියාචක", 50, side("applicant_side")},
		{"intervenient", "Intervenient petitioner", "මැදිහත් පෙත්සම්කරු", 55, side("neutral")},
		{"complainant", "Complainant", "පැමිණිලිකරු (අපරාධ)", 60, side("applicant_side")},
		{"accused", "Accused", "චූදිතයා", 70, side("respondent_side")},
		{"suspect", "Suspect", "සැකකරු", 75, side("respondent_side")},
		{"aggrieved_party", "Aggrieved party", "අගතියට පත් පාර්ශවය", 80, side("neutral")},
		{"applicant", "Applicant", "ඉල්ලුම්කරු", 85, side("applicant_side")},
		{"witness", "Witness", "සාක්ෂිකරු", 90, side("neutral")},
	},
	KindHearingPurpose: {
		{"mention", "Mention / calling", "කැඳවීම", 10, nil},
		{"answer", "Answer", "උත්තරය", 20, nil},
		{"replication", "Replication", "ප්\u200Dරතිඋත්තරය", 30, nil},
		{"pre_trial", "Pre-trial", "පූර්ව නඩු විභාගය", 40, nil},
		{"trial", "Trial", "නඩු විභාගය", 50, nil},
		{"further_trial", "Further trial", "වැඩිදුර නඩු විභාගය", 55, nil},
		{"inquiry", "Inquiry", "විමර්ශනය", 60, nil},
		{"support", "Support (leave / notice)", "සහාය දැක්වීම", 65, nil},
		{"argument", "Argument", "තර්ක", 70, nil},
		{"written_submissions", "Written submissions", "ලිඛිත දේශන", 75, nil},
		{"judgment", "Judgment", "තීන්දුව", 80, nil},
		{"order", "Order", "නියෝගය", 85, nil},
		{"bail", "Bail", "ඇප", 90, nil},
		{"remand_extension", "Remand extension", "රක්ෂිත බන්ධනාගාර දීර්ඝ කිරීම", 95, nil},
		{"settlement", "Settlement", "සමථය", 100, nil},
		{"other", "Other", "වෙනත්", 999, nil},
	},
	KindOutcomeType: {
		{"adjourned", "Adjourned", "කල් තැබුවා", 10, outcome(true, false)},
		{"step_completed", "Step completed", "පියවර සම්පූර්ණයි", 20, outcome(true, false)},
		{"evidence_led", "Evidence led", "සාක්ෂි මෙහෙයවූවා", 30, outcome(true, false)},
		{"argued", "Argued", "තර්ක කළා", 40, outcome(true, false)},
		{"order_made", "Order made", "නියෝගයක් කළා", 50, outcome(false, false)},
		{"judgment_reserved", "Judgment reserved", "තීන්දුව ලබා දීමට නියමිතයි", 60, outcome(true, false)},
		{"judgment_delivered", "Judgment delivered", "තීන්දුව ලබා දුන්නා", 70, outcome(false, true)},
		{"settled", "Settled", "සමථයකට පත් වුණා", 80, outcome(false, true)},
		{"withdrawn", "Withdrawn", "ඉල්ලා අස් කර ගත්තා", 90, outcome(false, true)},
		{"not_reached", "Not reached", "කැඳවූයේ නැත", 100, outcome(true, false)},
		{"court_not_sitting", "Court not sitting", "අධිකරණය රැස් නොවීය", 110, outcome(true, false)},
		{"bail_granted", "Bail granted", "ඇප ලබා දුන්නා", 120, outcome(true, false)},
		{"bail_refused", "Bail refused", "ඇප ප්\u200Dරතික්ෂේප කළා", 130, outcome(true, false)},
		{"remand_extended", "Remand extended", "රක්ෂිතය දීර්ඝ කළා", 140, outcome(true, false)},
		{"laid_by", "Laid by / sine die", "නියමිත දිනයක් නොමැතිව", 150, outcome(false, false)},
	},
	KindRelationshipType: {
		{"appeal_of", "Appeal of", "අභියාචනය", 10, rel(true, false)},
		{"leave_to_appeal_from", "Leave to appeal from", "අභියාචනා අවසරය", 20, rel(true, false)},
		{"appeal_after_leave", "Appeal after leave", "අවසරයෙන් පසු අභියාචනය", 30, rel(true, false)},
		{"revision_of", "Revision of", "ප්\u200Dරතිශෝධනය", 40, rel(true, false)},
		{"transferred_from", "Transferred from", "මාරු කළේ", 50, rel(true, false)},
		{"execution_of", "Execution of", "ක්\u200Dරියාත්මක කිරීම", 60, rel(false, false)},
		{"connected_with", "Connected with", "සම්බන්ධිත", 70, rel(false, true)},
		{"consolidated_with", "Consolidated with", "ඒකාබද්ධ කළ", 80, rel(false, true)},
	},
	KindDateSourceType: {
		{"case_record", "Case record / journal entry", "නඩු වාර්තාව / ජර්නල් සටහන", 10, authority("authoritative")},
		{"certified_order", "Certified copy of order", "සහතික කළ නියෝග පිටපත", 20, authority("authoritative")},
		{"official_court_list", "Official court list", "නිල නඩු ලැයිස්තුව", 30, authority("authoritative")},
		{"court_order", "Announced in court", "අධිකරණයේදී ප්\u200Dරකාශ කළ", 40, authority("primary")},
		{"lawyer_entry", "Lawyer entry", "නීතිඥ ඇතුළත් කිරීම", 50, authority("primary")},
		{"clerk_entry", "Clerk entry", "ලිපිකරු ඇතුළත් කිරීම", 55, authority("primary")},
		{"registry_in_person", "Registry (in person)", "රෙජිස්ට්\u200Dරාර් කාර්යාලය (පෞද්ගලිකව)", 60, authority("secondary")},
		{"registry_phone", "Registry (phone)", "රෙජිස්ට්\u200Dරාර් කාර්යාලය (දුරකථනය)", 65, authority("secondary")},
		{"court_notice", "Notice / summons from court", "අධිකරණ දැන්වීම / සිතාසිය", 70, authority("secondary")},
		{"email", "Email", "විද්\u200Dයුත් තැපෑල", 80, authority("secondary")},
		{"letter", "Letter / document", "ලිපිය / ලේඛනය", 85, authority("secondary")},
		{"opposing_counsel", "Opposing counsel", "විරුද්ධ පාර්ශවයේ නීතිඥ", 90, authority("hearsay")},
		{"client", "Client", "සේවාදායකයා", 95, authority("hearsay")},
		{"migrated", "Migrated from diary / spreadsheet", "පැරණි දිනපොතෙන් / පැතුරුම්පතෙන්", 100, authority("hearsay")},
	},
}

// SeedCourts is a starting list of courts around Colombo plus the appellate
// courts. The pilot firm adds the courts it attends (issue #30).
var SeedCourts = []SeedCourt{
	{"SC", "sc", "Supreme Court", "ශ්\u200Dරේෂ්ඨාධිකරණය", "Colombo 12", "කොළඹ 12", nil},
	{"CA", "ca", "Court of Appeal", "අභියාචනාධිකරණය", "Colombo 12", "කොළඹ 12", nil},
	{"HCCA-WP-COLOMBO", "hcca", "Civil Appellate High Court of the Western Province (Colombo)",
		"බස්නාහිර පළාත් සිවිල් අභියාචන මහාධිකරණය (කොළඹ)", "Colombo 12", "කොළඹ 12", nil},
	{"CHC-COLOMBO", "chc", "Commercial High Court of Colombo", "කොළඹ වාණිජ මහාධිකරණය", "Colombo 12", "කොළඹ 12", nil},
	{"HC-COLOMBO", "hc", "High Court of Colombo", "කොළඹ මහාධිකරණය", "Colombo 12", "කොළඹ 12", nil},
	{"DC-COLOMBO", "dc", "District Court of Colombo", "කොළඹ දිසා අධිකරණය", "Colombo 12", "කොළඹ 12", dcPatterns},
	{"DC-MT-LAVINIA", "dc", "District Court of Mount Lavinia", "ගල්කිස්ස දිසා අධිකරණය", "Mount Lavinia", "ගල්කිස්ස", dcPatterns},
	{"DC-GAMPAHA", "dc", "District Court of Gampaha", "ගම්පහ දිසා අධිකරණය", "Gampaha", "ගම්පහ", dcPatterns},
	{"DC-KANDY", "dc", "District Court of Kandy", "මහනුවර දිසා අධිකරණය", "Kandy", "මහනුවර", dcPatterns},
	{"MC-COLOMBO", "mc", "Chief Magistrate's Court of Colombo", "කොළඹ ප්\u200Dරධාන මහේස්ත්\u200Dරාත් අධිකරණය", "Colombo 12", "කොළඹ 12", mcPatterns},
	{"MC-COLOMBO-FORT", "mc", "Magistrate's Court of Colombo Fort", "කොටුව මහේස්ත්\u200Dරාත් අධිකරණය", "Colombo Fort", "කොටුව", mcPatterns},
	{"MC-MT-LAVINIA", "mc", "Magistrate's Court of Mount Lavinia", "ගල්කිස්ස මහේස්ත්\u200Dරාත් අධිකරණය", "Mount Lavinia", "ගල්කිස්ස", mcPatterns},
	{"MC-GAMPAHA", "mc", "Magistrate's Court of Gampaha", "ගම්පහ මහේස්ත්\u200Dරාත් අධිකරණය", "Gampaha", "ගම්පහ", mcPatterns},
	{"MC-KANDY", "mc", "Magistrate's Court of Kandy", "මහනුවර මහේස්ත්\u200Dරාත් අධිකරණය", "Kandy", "මහනුවර", mcPatterns},
}

// Illustrative patterns: they warn on unusual input and never block, because
// real numbering varies by court and year (doc 05 §3.1).
var (
	dcPatterns = []CaseNumberPattern{{Pattern: `^[A-Z]{1,5}/\d{1,6}(/\d{2,4})?$`, Example: "DMR/1234/24"}}
	mcPatterns = []CaseNumberPattern{{Pattern: `^(B/?)?\d{1,6}/\d{2,4}$`, Example: "B/1234/26"}}
)

func attrs(level, category string) map[string]any {
	return map[string]any{"court_level": level, "case_category": category}
}
func side(s string) map[string]any { return map[string]any{"side": s} }
func outcome(requiresNext, concludes bool) map[string]any {
	return map[string]any{"requires_next": requiresNext, "concludes": concludes}
}
func rel(derivation, symmetric bool) map[string]any {
	return map[string]any{"derivation": derivation, "symmetric": symmetric}
}
func authority(a string) map[string]any { return map[string]any{"default_authority": a} }
