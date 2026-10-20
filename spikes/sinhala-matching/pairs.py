"""Synthetic labelled name pairs (fabricated combinations of common Sri Lankan names; no real clients).
Each entry: (category, name_a, name_b, is_same_person_candidate)."""

ZWJ, AL = "‍", "්"

PAIRS = [
    # --- Cross-script: Sinhala vs English spelling of the same name -----------------------------
    ("cross-script", "නිමල් පෙරේරා", "Nimal Perera", True),
    ("cross-script", "සුනිල් ප්‍රනාන්දු", "Sunil Fernando", True),
    ("cross-script", "කමල් ද සිල්වා", "Kamal de Silva", True),
    ("cross-script", "චමරි ජයවර්ධන", "Chamari Jayawardena", True),
    ("cross-script", "රුවන් වික්‍රමසිංහ", "Ruwan Wickramasinghe", True),
    ("cross-script", "දිලානි ගුණසේකර", "Dilani Gunasekera", True),
    ("cross-script", "සඳුන් විජේසිංහ", "Sandun Wijesinghe", True),
    ("cross-script", "තිලිණි දිසානායක", "Thilini Dissanayake", True),
    ("cross-script", "කසුන් බණ්ඩාර", "Kasun Bandara", True),
    ("cross-script", "අනුෂා රත්නායක", "Anusha Rathnayake", True),
    ("cross-script", "ප්‍රියංකා ජයසිංහ", "Priyanka Jayasinghe", True),
    ("cross-script", "මහින්ද හේරත්", "Mahinda Herath", True),
    ("cross-script", "නාමල් රාජපක්ෂ", "Namal Rajapaksa", True),
    ("cross-script", "ලසන්ත සේනානායක", "Lasantha Senanayake", True),
    ("cross-script", "චාමික වීරසිංහ", "Chamika Weerasinghe", True),
    ("cross-script", "සමන් අබේසේකර", "Saman Abeysekera", True),
    ("cross-script", "නුවන් සමරවීර", "Nuwan Samaraweera", True),
    ("cross-script", "ඉරේෂා ලියනගේ", "Iresha Liyanage", True),
    ("cross-script", "අජිත් කරුණාරත්න", "Ajith Karunaratne", True),
    ("cross-script", "ශාන්ති ජයතිලක", "Shanthi Jayathilaka", True),
    ("cross-script", "රංජිත් සෙනෙවිරත්න", "Ranjith Seneviratne", True),
    ("cross-script", "උපුල් ධර්මසේන", "Upul Dharmasena", True),
    ("cross-script", "ගයාන් මෙන්ඩිස්", "Gayan Mendis", True),
    ("cross-script", "ඩබ්ලිව්. ඒ. පෙරේරා", "W.A. Perera", True),
    ("cross-script", "කේ. පී. ගමගේ", "K.P. Gamage", True),
    # --- English spelling variants ----------------------------------------------------------------
    ("spelling", "Ruwan Wickramasinghe", "Ruwan Wickremesinghe", True),
    ("spelling", "Ruwan Wickramasinghe", "Ruwan Wikramasinghe", True),
    ("spelling", "Chamari Jayawardena", "Chamari Jayawardene", True),
    ("spelling", "Chamari Jayawardena", "Chamari Jayewardene", True),
    ("spelling", "Nimal Perera", "Nimal Pereira", True),
    ("spelling", "Dilani Gunasekara", "Dilani Goonesekera", True),
    ("spelling", "Anusha Ratnayake", "Anusha Rathnayaka", True),
    ("spelling", "Namal Rajapaksa", "Namal Rajapakshe", True),
    ("spelling", "Saman Abeysekera", "Saman Abeyesekara", True),
    ("spelling", "Chamika Weerasinghe", "Chamika Virasinghe", True),
    ("spelling", "Shanthi Jayatilaka", "Shanthi Jayatilleke", True),
    ("spelling", "Thilini Dissanayake", "Thilini Disanayaka", True),
    ("spelling", "Ajith Karunaratne", "Ajith Karunaratna", True),
    ("spelling", "Sandun Wijesinghe", "Sandun Wijesinha", True),
    ("spelling", "Lasantha Senanayake", "Lasanta Senanayaka", True),
    # --- Initials and ordering --------------------------------------------------------------------
    ("initials", "W.A. Perera", "Perera W.A.", True),
    ("initials", "W.A. Perera", "Perera, W. A.", True),
    ("initials", "W A Perera", "Perera WA", True),
    ("initials", "K.P. Gamage", "Gamage K P", True),
    ("initials", "R.M. Bandara", "Bandara R.M.", True),
    ("initials", "Nimal Perera", "Perera Nimal", True),
    ("initials", "W.A. Perera", "Wanigasekara Arachchige Perera", True),
    ("initials", "H.M. Dissanayake", "Herath Mudiyanselage Dissanayake", True),
    # --- Sinhala typing variants (same name, different code-point sequences) -----------------------
    ("typing", "ප්" + ZWJ + "රනාන්දු", "ප" + ZWJ + AL + "රනාන්දු", True),           # ZWJ before al-lakuna
    ("typing", "ප්‍රනාන්දු", "ප්රනාන්දු", True),                                    # rakaransaya without ZWJ
    ("typing", "පෙරේරා", "ෙපරේරා", True),                               # kombuva typed first (visual order)
    ("typing", "ගමගේ", "ගමගේ", True),                                        # decomposed ee sign
    ("typing", "ජයතිලක", "ජය​තිලක", True),                                           # zero-width space
    ("typing", "ආරියරත්න", "අාරියරත්න", True),                                 # a + aa-sign for independent aa
    ("typing", "රෝහණ", "රෝහණ", True),                                    # decomposed oo sign
    # --- True non-matches --------------------------------------------------------------------------
    ("non-match", "Nimal Perera", "Sunil Perera", False),
    ("non-match", "Nimal Perera", "Nimal Fernando", False),
    ("non-match", "Kasun Jayasinghe", "Kasun Jayasekara", False),
    ("non-match", "Sandun Wijesinghe", "Sandun Wijeratne", False),
    ("non-match", "Anusha Kumara", "Anusha Kumari", False),
    ("non-match", "Kasun Bandara", "Kasun Bandaranayake", False),
    ("non-match", "Dilani Gunasekara", "Dilani Gunawardena", False),
    ("non-match", "Thilini Dissanayake", "Thilini Senanayake", False),
    ("non-match", "Gayan Ranasinghe", "Gayan Munasinghe", False),
    ("non-match", "Ruwan Wickramasinghe", "Ruwan Wickramaratne", False),
    ("non-match", "W.A. Perera", "K.P. Perera", False),
    ("non-match", "R.M. Bandara", "R.M. Bandaranayake", False),
    ("non-match", "Saman Kumara", "Kamal Kumara", False),
    ("non-match", "Chamari Jayawardena", "Chamari Jayasundara", False),
    ("non-match", "Nuwan Samaraweera", "Nuwan Samarakoon", False),
    ("non-match", "Mahinda Herath", "Mahinda Hettiarachchi", False),
    ("non-match", "නිමල් පෙරේරා", "Sunil Perera", False),
    ("non-match", "කසුන් ජයසිංහ", "Kasun Jayasekara", False),
    ("non-match", "ගයාන් රණසිංහ", "Gayan Munasinghe", False),
    ("non-match", "අජිත් කරුණාරත්න", "Ajith Karunanayake", False),
    ("non-match", "Upul Dharmasena", "Upul Dharmadasa", False),
    ("non-match", "Lasantha Silva", "Lasantha Silvapulle", False),
]
